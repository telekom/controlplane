// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Listener Drain", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *handler.ListenerHandler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &handler.ListenerHandler{}

		scheme = runtime.NewScheme()
		_ = spectrev1.AddToScheme(scheme)
		_ = approvalv1.AddToScheme(scheme)
		_ = gatewayv1.AddToScheme(scheme)
		_ = pubsubv1.AddToScheme(scheme)
		fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
	})

	// --- Multi-RouteListener drain helpers ---

	// drainZoneB is a second capture zone namespace; it sorts after listenerZoneStatus.
	const drainZoneB = "env-ns--cetus"
	const (
		rlKind  = "RouteListener"
		subKind = "Subscriber"
	)

	// persistListener round-trips l through JSON. It stands in for the status
	// write and re-read between reconciles and proves the drain fields survive.
	persistListener := func(l *spectrev1.Listener) *spectrev1.Listener {
		data, err := json.Marshal(l)
		Expect(err).ToNot(HaveOccurred())
		out := &spectrev1.Listener{}
		Expect(json.Unmarshal(data, out)).To(Succeed())
		return out
	}

	// expectLive stubs one Get of ns/name that finds a live kind object with the
	// given UID and resourceVersion, optionally terminating behind a finalizer.
	expectLive := func(kind, ns, name string, uid k8stypes.UID, rv string, terminating bool) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: ns}, mock.AnythingOfType("*v1."+kind)).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				out.SetName(name)
				out.SetNamespace(ns)
				out.SetUID(uid)
				out.SetResourceVersion(rv)
				if terminating {
					now := metav1.Now()
					out.SetDeletionTimestamp(&now)
					out.SetFinalizers([]string{"test.cp.ei.telekom.de/hold"})
				}
			}).
			Return(nil).Once()
	}

	// expectGone stubs one Get of ns/name that returns NotFound.
	expectGone := func(kind, ns, name string) {
		gr := schema.GroupResource{Group: gatewayv1.GroupVersion.Group, Resource: "routelisteners"}
		if kind == subKind {
			gr = schema.GroupResource{Group: pubsubv1.GroupVersion.Group, Resource: "subscribers"}
		}
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: ns}, mock.AnythingOfType("*v1."+kind)).
			Return(errors.NewNotFound(gr, name)).Once()
	}

	// expectDelete stubs one Delete of the kind object ns/name. It only matches
	// when the call carries exactly the given UID and resourceVersion
	// preconditions; the strict mock fails any other Delete.
	expectDelete := func(kind, ns, name string, uid k8stypes.UID, rv string, result error) {
		fakeClient.EXPECT().
			Delete(ctx,
				mock.MatchedBy(func(obj client.Object) bool {
					return reflect.TypeOf(obj).Elem().Name() == kind && obj.GetNamespace() == ns && obj.GetName() == name
				}),
				mock.MatchedBy(func(p client.Preconditions) bool {
					return p.UID != nil && *p.UID == uid && p.ResourceVersion != nil && *p.ResourceVersion == rv
				})).
			Return(result).Once()
	}

	// expectPassDone asserts that every stub registered so far was consumed and
	// that exactly deletes Delete calls were made since the test started.
	expectPassDone := func(deletes int) {
		fakeClient.AssertExpectations(GinkgoT())
		fakeClient.AssertNumberOfCalls(GinkgoT(), "Delete", deletes)
	}

	// mockDrainLists stubs the startDrain owner-label Lists of RouteListeners
	// and Subscribers.
	mockDrainLists := func(rls []gatewayv1.RouteListener, subs []pubsubv1.Subscriber) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{Items: rls}
			}).
			Return(nil).Once()
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{Items: subs}
			}).
			Return(nil).Once()
	}

	liveRL := func(ns, name string, uid k8stypes.UID) gatewayv1.RouteListener {
		return gatewayv1.RouteListener{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid}}
	}
	liveSub := func(ns, name string, uid k8stypes.UID) pubsubv1.Subscriber {
		return pubsubv1.Subscriber{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid}}
	}

	// twoRouteListenerCheckpoint is a Stopping checkpoint written by this build:
	// rl-a (also the status ref) and rl-b, an orphan in another zone.
	twoRouteListenerCheckpoint := func() *spectrev1.Listener {
		listener := newListener()
		listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}
		listener.Status.Draining = &spectrev1.ListenerDrainStatus{
			Phase:            handler.ExportDrainPhaseStopping,
			OldFingerprint:   "old-fp",
			OldRouteListener: &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"},
			OldRouteListeners: []ctypes.ObjectRef{
				{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"},
				{Name: "rl-b", Namespace: drainZoneB, UID: "uid-b"},
			},
			OldSubscribers: []ctypes.ObjectRef{{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-sub"}},
		}
		return listener
	}

	Describe("StartDrain", func() {
		It("should snapshot status refs, discover owner-labelled children, and record drain", func() {
			listener := newListener()
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
			listener.Status.EventSubscriptions = []ctypes.ObjectRef{
				{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				{Name: "old-sub-rp", Namespace: listenerZoneStatus},
			}
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
				Fingerprint:       "old-fp-abc",
				Publisher:         &ctypes.ObjectRef{Name: "pub-generic", Namespace: listenerZoneStatus},
				CaptureEventStore: &ctypes.ObjectRef{Name: "es-capture", Namespace: listenerZoneStatus},
			}

			// Owner-labelled discovery returns the same children (no extras).
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
						Items: []gatewayv1.RouteListener{
							{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-1"}},
						},
					}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus, UID: "sub-uid-1"}},
							{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rp", Namespace: listenerZoneStatus, UID: "sub-uid-2"}},
						},
					}
				}).
				Return(nil).Once()

			err := h.StartDrain(ctx, listener, "fingerprint changed", "old-fp-abc")
			Expect(err).ToNot(HaveOccurred())

			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			Expect(listener.Status.Draining.Reason).To(Equal("fingerprint changed"))
			Expect(listener.Status.Draining.OldFingerprint).To(Equal("old-fp-abc"))
			Expect(listener.Status.Draining.OldRouteListener).ToNot(BeNil())
			Expect(listener.Status.Draining.OldRouteListener.Name).To(Equal("old-rl"))
			Expect(listener.Status.Draining.OldRouteListener.UID).To(Equal(k8stypes.UID("rl-uid-1")))
			Expect(listener.Status.Draining.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
				{Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-1"},
			}))
			// Deduped by namespace/name, UIDs filled from the live objects, sorted.
			Expect(listener.Status.Draining.OldSubscribers).To(Equal([]ctypes.ObjectRef{
				{Name: "old-sub-rp", Namespace: listenerZoneStatus, UID: "sub-uid-2"},
				{Name: "old-sub-rq", Namespace: listenerZoneStatus, UID: "sub-uid-1"},
			}))
			// SourcePublisher and SourceEventStore are recorded.
			Expect(listener.Status.Draining.SourcePublisher).ToNot(BeNil())
			Expect(listener.Status.Draining.SourcePublisher.Name).To(Equal("pub-generic"))
			Expect(listener.Status.Draining.SourceEventStore).ToNot(BeNil())
			Expect(listener.Status.Draining.SourceEventStore.Name).To(Equal("es-capture"))
		})

		It("should not call Delete — startDrain only snapshots", func() {
			listener := newListener()
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()

			err := h.StartDrain(ctx, listener, "test", "old-fp")
			Expect(err).ToNot(HaveOccurred())
			// No Delete calls were made — mockery strict mode would fail if any were.
			Expect(listener.Status.Draining).ToNot(BeNil())
		})

		It("should handle nil status refs gracefully", func() {
			listener := newListener()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()

			err := h.StartDrain(ctx, listener, "test", "old-fp")
			Expect(err).ToNot(HaveOccurred())

			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.OldRouteListener).To(BeNil())
			Expect(listener.Status.Draining.OldRouteListeners).To(BeNil())
			Expect(listener.Status.Draining.OldSubscribers).To(BeNil())
			Expect(listener.Status.Draining.SourcePublisher).To(BeNil())
		})

		It("should discover extra owner-labelled children not in status refs", func() {
			listener := newListener()
			// Status has no children, but owner-labelled discovery finds one.

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
						Items: []gatewayv1.RouteListener{
							{ObjectMeta: metav1.ObjectMeta{Name: "orphan-rl", Namespace: listenerZoneStatus, UID: "orphan-uid"}},
						},
					}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{ObjectMeta: metav1.ObjectMeta{Name: "orphan-sub", Namespace: listenerZoneStatus, UID: "orphan-sub-uid"}},
						},
					}
				}).
				Return(nil).Once()

			err := h.StartDrain(ctx, listener, "test", "old-fp")
			Expect(err).ToNot(HaveOccurred())

			Expect(listener.Status.Draining.OldRouteListener).ToNot(BeNil())
			Expect(listener.Status.Draining.OldRouteListener.Name).To(Equal("orphan-rl"))
			Expect(listener.Status.Draining.OldRouteListener.UID).To(Equal(k8stypes.UID("orphan-uid")))
			Expect(listener.Status.Draining.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
				{Name: "orphan-rl", Namespace: listenerZoneStatus, UID: "orphan-uid"},
			}))
			Expect(listener.Status.Draining.OldSubscribers).To(HaveLen(1))
			Expect(listener.Status.Draining.OldSubscribers[0].Name).To(Equal("orphan-sub"))
		})
	})

	Describe("Finding 3 regression: post-drain denial stability", func() {
		It("should not start a new drain when AppliedPlacement has empty fingerprint and no children", func() {
			listener := newListener()
			// Post-drain state: AppliedPlacement remains with empty fingerprint,
			// status refs cleared, Draining is nil.
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
				Fingerprint: "",
			}
			listener.Status.RouteListener = nil
			listener.Status.EventSubscriptions = nil

			err := h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())

			// No drain should have started.
			Expect(listener.Status.Draining).To(BeNil(),
				"No drain should start when children are already gone")
			// AppliedPlacement should be cleared.
			Expect(listener.Status.AppliedPlacement).To(BeNil(),
				"Stale AppliedPlacement should be cleared")
		})

		It("should start drain when AppliedPlacement has children to drain", func() {
			listener := newListener()
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
				Fingerprint: "live-fp",
			}
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-1", Namespace: listenerZoneStatus}
			listener.Status.EventSubscriptions = []ctypes.ObjectRef{
				{Name: "sub-1", Namespace: listenerZoneStatus},
			}

			// startDrain needs List mocks for snapshot.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()

			err := h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())

			// Drain should have started because children exist.
			Expect(listener.Status.Draining).ToNot(BeNil(),
				"Drain must start when children exist")
		})

		It("should be stable across repeated calls when already denied and drained", func() {
			listener := newListener()

			// Call 1: AppliedPlacement with empty FP, no children.
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
				Fingerprint: "",
			}

			err := h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())
			Expect(listener.Status.AppliedPlacement).To(BeNil())
			Expect(listener.Status.Draining).To(BeNil())

			// Call 2: Same state again — AppliedPlacement nil means we enter
			// the direct-delete branch, not the drain branch. No new drain starts.
			// This proves the cycle is broken: previously the code would restart
			// a drain on every reconcile.
			Expect(listener.Status.AppliedPlacement).To(BeNil(),
				"After clearing, repeated calls should not re-create AppliedPlacement")
			Expect(listener.Status.Draining).To(BeNil(),
				"No drain should be active after clearing")
		})
	})

	Describe("ContinueDrain", func() {
		It("should return true when draining is nil", func() {
			listener := newListener()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
		})

		It("should delete old RouteListener and requeue in Stopping phase", func() {
			listener := newListener()
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				Reason:         "fingerprint changed",
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "rl-uid-1",
				},
			}

			// Old RouteListener still exists — continueDrain deletes it.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					rl := out.(*gatewayv1.RouteListener)
					rl.Name = "old-rl"
					rl.Namespace = listenerZoneStatus
					rl.UID = "rl-uid-1"
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Requeue to verify deletion
		})

		It("should advance to DrainingSubscribers when RouteListener is gone", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
				},
			}

			// Old RouteListener is already gone.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Return(errors.NewNotFound(
					schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "routelisteners"}, "old-rl")).
				Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Phase advanced, persist
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
		})

		It("should treat different-UID RouteListener as gone", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "original-uid",
				},
			}

			// Object exists but with a different UID — someone recreated it.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					rl := out.(*gatewayv1.RouteListener)
					rl.Name = "old-rl"
					rl.Namespace = listenerZoneStatus
					rl.UID = "different-uid" // Not the UID we recorded
				}).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Phase advanced
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
		})

		It("should delete Subscribers and requeue in DrainingSubscribers phase", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseDrainingSubscribers,
				OldFingerprint: "old-fp",
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "old-sub", Namespace: listenerZoneStatus, UID: "sub-uid-1"},
				},
			}

			// Old Subscriber still exists.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-sub", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.Subscriber")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					sub := out.(*pubsubv1.Subscriber)
					sub.Name = "old-sub"
					sub.Namespace = listenerZoneStatus
					sub.UID = "sub-uid-1"
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Requeue to wait for finalization
		})

		It("should advance to CleaningPublisher when all Subscribers are gone", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseDrainingSubscribers,
				OldFingerprint: "old-fp",
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "old-sub", Namespace: listenerZoneStatus},
				},
			}

			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-sub", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.Subscriber")).
				Return(errors.NewNotFound(
					schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "subscribers"}, "old-sub")).
				Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Phase advanced, persist
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			Expect(listener.Status.EventSubscriptions).To(BeNil())
		})

		It("should treat different-UID Subscriber as gone", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseDrainingSubscribers,
				OldFingerprint: "old-fp",
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "old-sub", Namespace: listenerZoneStatus, UID: "original-sub-uid"},
				},
			}

			// Object exists but with different UID.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-sub", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.Subscriber")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					sub := out.(*pubsubv1.Subscriber)
					sub.Name = "old-sub"
					sub.Namespace = listenerZoneStatus
					sub.UID = "different-sub-uid"
				}).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Phase advanced
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
		})

		It("should delete Publisher when no Subscribers remain in CleaningPublisher", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:           handler.ExportDrainPhaseCleaningPublisher,
				OldFingerprint:  "old-fp",
				SourcePublisher: &ctypes.ObjectRef{Name: "pub-generic", Namespace: listenerZoneStatus},
			}

			// cleanupGenericPublisherIfOrphaned: list Subscribers (none) → delete Publisher.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})

		It("should preserve Publisher when other Subscribers exist in CleaningPublisher", func() {
			listener := newListener()
			genericPubName := util.MakePublisherName(util.GenericEventType)
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:           handler.ExportDrainPhaseCleaningPublisher,
				OldFingerprint:  "old-fp",
				SourcePublisher: &ctypes.ObjectRef{Name: genericPubName, Namespace: listenerZoneStatus},
			}

			// cleanupGenericPublisherIfOrphaned: a Subscriber still references the Publisher.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
						Items: []pubsubv1.Subscriber{
							{
								ObjectMeta: metav1.ObjectMeta{Name: "other-sub", Namespace: listenerZoneStatus},
								Spec: pubsubv1.SubscriberSpec{
									Publisher: ctypes.ObjectRef{Name: genericPubName, Namespace: listenerZoneStatus},
								},
							},
						},
					}
				}).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})

		It("should skip Publisher cleanup when SourcePublisher is nil", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseCleaningPublisher,
				OldFingerprint: "old-fp",
			}

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})

		It("should complete from Complete phase", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseComplete,
				OldFingerprint: "old-fp",
			}

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})

		It("should survive restart: re-enter with persisted Draining status", func() {
			listener := newListener()
			// Simulate: controller restarted while Stopping; old RL is now gone.
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				Reason:         "fingerprint changed",
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "rl-uid-1",
				},
			}

			// On restart, old RL is gone now.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Return(errors.NewNotFound(
					schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "routelisteners"}, "old-rl")).
				Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse()) // Phase advanced to DrainingSubscribers
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
		})
	})

	Describe("Multi-RouteListener drain", func() {
		Describe("StartDrain inventory", func() {
			It("should record the status RouteListener and every owner-labelled RouteListener across namespaces, sorted", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus}

				// Deliberately unsorted: rl-b is an orphan in another zone left by a
				// partial provisioning (created, but the status write failed).
				mockDrainLists([]gatewayv1.RouteListener{
					liveRL(drainZoneB, "rl-b", "uid-b"),
					liveRL(listenerZoneStatus, "rl-a", "uid-a"),
				}, nil)

				Expect(h.StartDrain(ctx, listener, "test", "old-fp")).To(Succeed())

				d := listener.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
					{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"},
					{Name: "rl-b", Namespace: drainZoneB, UID: "uid-b"},
				}))
				Expect(d.OldRouteListener).To(Equal(&ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}))
				Expect(d.OldSubscribers).To(BeNil())
				expectPassDone(0)
			})

			It("should keep both instances when the status UID differs from the live owner-labelled UID", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-stale"}

				mockDrainLists([]gatewayv1.RouteListener{liveRL(listenerZoneStatus, "rl-a", "uid-live")}, nil)

				Expect(h.StartDrain(ctx, listener, "test", "old-fp")).To(Succeed())

				Expect(listener.Status.Draining.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
					{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-live"},
					{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-stale"},
				}))
				expectPassDone(0)
			})

			It("should dedupe and sort Subscribers from status refs and owner-labelled discovery", func() {
				listener := newListener()
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "sub-rq", Namespace: listenerZoneStatus},
					{Name: "sub-rp", Namespace: listenerZoneStatus},
				}

				mockDrainLists(nil, []pubsubv1.Subscriber{
					liveSub(drainZoneB, "sub-x", "uid-3"),
					liveSub(listenerZoneStatus, "sub-rq", "uid-1"),
					liveSub(listenerZoneStatus, "sub-rp", "uid-2"),
				})

				Expect(h.StartDrain(ctx, listener, "test", "old-fp")).To(Succeed())

				Expect(listener.Status.Draining.OldSubscribers).To(Equal([]ctypes.ObjectRef{
					{Name: "sub-rp", Namespace: listenerZoneStatus, UID: "uid-2"},
					{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-1"},
					{Name: "sub-x", Namespace: drainZoneB, UID: "uid-3"},
				}))
				Expect(listener.Status.Draining.OldRouteListeners).To(BeNil())
				Expect(listener.Status.Draining.OldRouteListener).To(BeNil())
				expectPassDone(0)
			})
		})

		Describe("ContinueDrain Stopping", func() {
			It("should delete every recorded RouteListener in one Stopping pass and advance only after all are gone", func() {
				listener := twoRouteListenerCheckpoint()

				// P1: both still exist and are both deleted in the same pass.
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", false)
				expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", nil)
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "21", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "21", nil)
				done, err := h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(2)

				// P2: rl-a is gone, rl-b is still terminating: it is deleted again with
				// its fresh resourceVersion and the phase does not advance.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "22", true)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "22", nil)
				done, err = h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.RouteListener).ToNot(BeNil())
				expectPassDone(3)

				// P3: both are gone. Only now does the drain advance.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				expectGone(rlKind, drainZoneB, "rl-b")
				done, err = h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.Draining.OldSubscribers).To(HaveLen(1))
				expectPassDone(3)
				// Only RouteListener reads: no Subscriber was touched while stopping.
				fakeClient.AssertNumberOfCalls(GinkgoT(), "Get", 6)
			})

			It("should not delete a same-name replacement while another recorded RouteListener remains", func() {
				listener := twoRouteListenerCheckpoint()

				// P1: rl-a now has a different UID (a replacement); only rl-b is deleted.
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-new", "31", false)
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "21", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "21", nil)
				done, err := h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(1)

				// P2: the replacement is still there, rl-b is gone: the drain advances
				// without ever deleting the replacement.
				listener = persistListener(listener)
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-new", "31", false)
				expectGone(rlKind, drainZoneB, "rl-b")
				done, err = h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				expectPassDone(1)
			})

			It("should attempt every recorded RouteListener and return read and delete errors without advancing", func() {
				listener := twoRouteListenerCheckpoint()

				// P1: the rl-a read fails and the rl-b delete fails. Both are attempted.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "rl-a", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.RouteListener")).
					Return(errors.NewServiceUnavailable("etcd")).Once()
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "21", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "21", errors.NewServiceUnavailable("etcd"))
				done, err := h.ContinueDrain(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(listenerZoneStatus + "/rl-a"))
				Expect(err.Error()).To(ContainSubstring(drainZoneB + "/rl-b"))
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.RouteListener).ToNot(BeNil())
				expectPassDone(1)

				// P2: the API recovers; both are deleted, still Stopping.
				listener = persistListener(listener)
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "12", false)
				expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "12", nil)
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "22", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "22", nil)
				done, err = h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(3)

				// P3: both are gone; the drain advances.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				expectGone(rlKind, drainZoneB, "rl-b")
				done, err = h.ContinueDrain(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(done).To(BeFalse())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				expectPassDone(3)
			})
		})

		Describe("through CreateOrUpdate", func() {
			It("should drain a legacy checkpoint that only has the singular OldRouteListener", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus, UID: "uid-old"}
				listener.Status.Draining = &spectrev1.ListenerDrainStatus{
					Phase:            handler.ExportDrainPhaseStopping,
					OldFingerprint:   "old-fp",
					OldRouteListener: &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus, UID: "uid-old"},
				}

				// R1: the recorded RouteListener exists and is deleted.
				expectLive(rlKind, listenerZoneStatus, "old-rl", "uid-old", "5", false)
				expectDelete(rlKind, listenerZoneStatus, "old-rl", "uid-old", "5", nil)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(1)

				// R2: it is gone; the drain advances and the status ref is cleared.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "old-rl")
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				Expect(listener.Status.RouteListener).To(BeNil())
				expectPassDone(1)
			})

			It("should drain the status RouteListener a legacy single-ref checkpoint dropped", func() {
				// The previous build kept only the last listed RouteListener (rl-b) and
				// lost the status ref (rl-a).
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}
				listener.Status.Draining = &spectrev1.ListenerDrainStatus{
					Phase:            handler.ExportDrainPhaseStopping,
					OldFingerprint:   "old-fp",
					OldRouteListener: &ctypes.ObjectRef{Name: "rl-b", Namespace: drainZoneB, UID: "uid-b"},
				}

				// R1: both exist and both are deleted.
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "21", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "21", nil)
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", false)
				expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", nil)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(2)

				// R2: both are gone; the drain advances.
				listener = persistListener(listener)
				expectGone(rlKind, drainZoneB, "rl-b")
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				Expect(listener.Status.RouteListener).To(BeNil())
				expectPassDone(2)
			})

			It("should record and drain a status RouteListener and an orphan in another zone before touching Subscribers", func() {
				providerApproval := k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace}
				expectApprovalRejected := func() {
					fakeClient.EXPECT().
						Get(ctx, providerApproval, mock.AnythingOfType("*v1.Approval")).
						Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
							out.(*approvalv1.Approval).Spec.State = approvalv1.ApprovalStateRejected
						}).
						Return(nil).Once()
				}
				expectAccessDenied := func(l *spectrev1.Listener) {
					ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
					Expect(ready).ToNot(BeNil())
					Expect(ready.Status).To(Equal(metav1.ConditionFalse))
					Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
				}

				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{Name: providerApproval.Name, Namespace: providerApproval.Namespace}
				listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{Fingerprint: "fp-old"}
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-sub"}}

				// R1: the provider Approval is Rejected. The drain checkpoint records the
				// status RouteListener and rl-b, an orphan in another zone from a partial
				// provisioning. Nothing is deleted yet.
				expectApprovalRejected()
				mockDrainLists(
					[]gatewayv1.RouteListener{liveRL(listenerZoneStatus, "rl-a", "uid-a"), liveRL(drainZoneB, "rl-b", "uid-b")},
					[]pubsubv1.Subscriber{liveSub(listenerZoneStatus, "sub-rq", "uid-sub")},
				)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				d := listener.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.OldFingerprint).To(Equal("fp-old"))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
					{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"},
					{Name: "rl-b", Namespace: drainZoneB, UID: "uid-b"},
				}))
				Expect(d.OldRouteListener).To(Equal(&ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}))
				Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-sub"}}))
				expectAccessDenied(listener)
				expectPassDone(0)

				// R2: both RouteListeners are deleted in one pass.
				listener = persistListener(listener)
				expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", false)
				expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", nil)
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "21", false)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "21", nil)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.RouteListener).To(Equal(&ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}))
				expectPassDone(2)

				// R3: rl-a is gone, rl-b is terminating and deleted again.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "22", true)
				expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "22", nil)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				expectPassDone(3)

				// R4: both are gone; the drain advances to Subscribers.
				listener = persistListener(listener)
				expectGone(rlKind, listenerZoneStatus, "rl-a")
				expectGone(rlKind, drainZoneB, "rl-b")
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(HaveLen(1))
				expectPassDone(3)

				// R5: the Subscriber is deleted with its preconditions.
				listener = persistListener(listener)
				expectLive(subKind, listenerZoneStatus, "sub-rq", "uid-sub", "41", false)
				expectDelete(subKind, listenerZoneStatus, "sub-rq", "uid-sub", "41", nil)
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
				expectPassDone(4)

				// R6: the Subscriber is gone.
				listener = persistListener(listener)
				expectGone(subKind, listenerZoneStatus, "sub-rq")
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
				expectPassDone(4)

				// R7: the drain completes (no source Publisher recorded) and the same
				// reconcile re-reads the still-Rejected Approval, which clears the
				// drained applied placement instead of starting another drain.
				listener = persistListener(listener)
				expectApprovalRejected()
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining).To(BeNil())
				Expect(listener.Status.AppliedPlacement).To(BeNil())
				expectAccessDenied(listener)
				// rl-a once, rl-b twice, sub-rq once; no Publisher delete.
				expectPassDone(4)
			})
		})
	})
})
