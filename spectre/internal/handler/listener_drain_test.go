// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"reflect"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
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

	genericPublisher := util.MakePublisherName(util.GenericEventType)

	// expectOrphanCheck stubs the CleaningPublisher Subscriber List in ns,
	// returning subs.
	expectOrphanCheck := func(ns string, subs []pubsubv1.Subscriber) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), client.InNamespace(ns)).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{Items: subs}
			}).
			Return(nil).Once()
	}

	// expectPublisherDelete stubs one Delete of the generic Publisher in ns; the
	// strict mock fails a Publisher Delete in any other namespace.
	expectPublisherDelete := func(ns string) {
		fakeClient.EXPECT().
			Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
				_, ok := obj.(*pubsubv1.Publisher)
				return ok && obj.GetNamespace() == ns && obj.GetName() == genericPublisher
			})).
			Return(nil).Once()
	}

	// bridgeSub is a Subscriber in ns referencing the generic Publisher there.
	bridgeSub := func(ns, name string, terminating bool) pubsubv1.Subscriber {
		sub := pubsubv1.Subscriber{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       pubsubv1.SubscriberSpec{Publisher: ctypes.ObjectRef{Name: genericPublisher, Namespace: ns}},
		}
		if terminating {
			now := metav1.Now()
			sub.DeletionTimestamp = &now
			sub.Finalizers = []string{"pubsub.cp.ei.telekom.de/finalizer"}
		}
		return sub
	}

	// twoRouteListenerCheckpoint is a Stopping checkpoint written by this build:
	// rl-a (also the status ref) and rl-b, an orphan in another zone.
	twoRouteListenerCheckpoint := func() *spectrev1.Listener {
		listener := newListener()
		listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}
		listener.Status.Draining = &spectrev1.ListenerDrainStatus{
			Phase:          handler.ExportDrainPhaseStopping,
			OldFingerprint: "old-fp",
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

			// The owner-label inventory finds no untracked children either.
			mockDrainLists(nil, nil)

			err := h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())
			// No child and no Publisher is deleted.
			fakeClient.AssertNumberOfCalls(GinkgoT(), "Delete", 0)

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

			mockDrainLists(nil, nil)
			err := h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())
			Expect(listener.Status.AppliedPlacement).To(BeNil())
			Expect(listener.Status.Draining).To(BeNil())

			// Call 2: Same state again — the inventory is empty and nothing is
			// applied, so no drain starts and nothing is deleted. This proves the
			// cycle is broken: previously the code would restart a drain on every
			// reconcile.
			mockDrainLists(nil, nil)
			err = h.HandleDenialCleanup(ctx, listener, "provider")
			Expect(err).ToNot(HaveOccurred())
			Expect(listener.Status.AppliedPlacement).To(BeNil(),
				"After clearing, repeated calls should not re-create AppliedPlacement")
			Expect(listener.Status.Draining).To(BeNil(),
				"No drain should be active after clearing")
			fakeClient.AssertNumberOfCalls(GinkgoT(), "Delete", 0)
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
				OldRouteListeners: []ctypes.ObjectRef{{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "rl-uid-1",
				}},
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
				OldRouteListeners: []ctypes.ObjectRef{{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
				}},
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
				OldRouteListeners: []ctypes.ObjectRef{{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "original-uid",
				}},
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

		It("should skip Publisher cleanup when neither a SourcePublisher nor old Subscribers are recorded", func() {
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

		It("should clean every old Subscriber namespace and advance only when all succeed", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseCleaningPublisher,
				OldFingerprint: "old-fp",
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "sub-a", Namespace: listenerZoneStatus},
					{Name: "sub-b", Namespace: drainZoneB},
				},
			}

			// Pass 1: the orphan check in the first namespace fails; the second is
			// still attempted and its orphaned Publisher deleted.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), client.InNamespace(listenerZoneStatus)).
				Return(errors.NewServiceUnavailable("etcd leader changed")).Once()
			expectOrphanCheck(drainZoneB, nil)
			expectPublisherDelete(drainZoneB)
			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).To(MatchError(ContainSubstring("etcd leader changed")))
			Expect(done).To(BeFalse())
			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			expectPassDone(1)

			// Pass 2: both succeed; the drain completes.
			expectOrphanCheck(listenerZoneStatus, nil)
			expectPublisherDelete(listenerZoneStatus)
			expectOrphanCheck(drainZoneB, nil)
			expectPublisherDelete(drainZoneB)
			done, err = h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
			expectPassDone(3)
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
				OldRouteListeners: []ctypes.ObjectRef{{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
					UID:       "rl-uid-1",
				}},
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

				// R7: no source Publisher was recorded, so the drain cleans the orphaned
				// Publisher in the old Subscriber's namespace and completes. The same
				// reconcile re-reads the still-Rejected Approval, which clears the
				// drained applied placement instead of starting another drain.
				listener = persistListener(listener)
				Expect(listener.Status.Draining.SourcePublisher).To(BeNil())
				expectOrphanCheck(listener.Status.Draining.OldSubscribers[0].Namespace, nil)
				expectPublisherDelete(listenerZoneStatus)
				expectApprovalRejected()
				mockDrainLists(nil, nil) // drainCapture inventory: nothing left
				Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
				Expect(listener.Status.Draining).To(BeNil())
				Expect(listener.Status.AppliedPlacement).To(BeNil())
				expectAccessDenied(listener)
				// rl-a once, rl-b twice, sub-rq once, the Publisher once.
				expectPassDone(5)
			})

			DescribeTable("should clean the Publisher in every unreferenced old namespace and keep it where another Subscriber references it",
				func(source *ctypes.ObjectRef) {
					providerApproval := k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace}
					listener := newListener()
					listener.Status.ProviderApproval = &ctypes.ObjectRef{Name: providerApproval.Name, Namespace: providerApproval.Namespace}
					listener.Status.Draining = &spectrev1.ListenerDrainStatus{
						Phase:           handler.ExportDrainPhaseCleaningPublisher,
						OldFingerprint:  "fp-old",
						SourcePublisher: source,
						OldSubscribers: []ctypes.ObjectRef{
							{Name: "sub-rq", Namespace: drainZoneB, UID: "uid-b"},
							{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-a"},
						},
					}

					// listenerZoneStatus: another observer's terminating bridge still
					// references the Publisher, so it is kept. drainZoneB: no reference
					// remains, so it is deleted there, and only there.
					expectOrphanCheck(listenerZoneStatus, []pubsubv1.Subscriber{bridgeSub(listenerZoneStatus, "other-sub", true)})
					expectOrphanCheck(drainZoneB, []pubsubv1.Subscriber{{
						ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: drainZoneB},
						Spec:       pubsubv1.SubscriberSpec{Publisher: ctypes.ObjectRef{Name: "other-publisher", Namespace: drainZoneB}},
					}})
					expectPublisherDelete(drainZoneB)
					// The drain completes and the reconcile falls through to the still
					// Rejected Approval, which starts no new drain.
					fakeClient.EXPECT().
						Get(ctx, providerApproval, mock.AnythingOfType("*v1.Approval")).
						Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
							out.(*approvalv1.Approval).Spec.State = approvalv1.ApprovalStateRejected
						}).
						Return(nil).Once()
					mockDrainLists(nil, nil)

					Expect(h.CreateOrUpdate(ctx, listener)).To(Succeed())
					Expect(listener.Status.Draining).To(BeNil())
					expectPassDone(1)
				},
				Entry("without a recorded source Publisher", nil),
				Entry("with the source Publisher recorded in one of them",
					&ctypes.ObjectRef{Name: util.MakePublisherName(util.GenericEventType), Namespace: listenerZoneStatus}),
			)
		})
	})

	// Delete runs under the common controller, which removes the finalizer only
	// when Delete returns nil and writes status only when it returns an error.
	// Every pass that must be persisted therefore returns a retryable error.
	Describe("through Delete", func() {
		// expectDrainPending asserts err keeps the finalizer and asks for a
		// delayed retry with status persisted (the common controller's
		// RetryableWithDelayError handling).
		expectDrainPending := func(err error) {
			var rde ctrlerrors.RetryableWithDelayError
			Expect(stderrors.As(err, &rde)).To(BeTrue(), "want a retryable-with-delay error, got %v", err)
			Expect(rde.IsRetryable()).To(BeTrue())
			Expect(rde.RetryDelay()).To(BeNumerically(">", 0))
		}
		genericIn := func(ns string) *ctypes.ObjectRef {
			return &ctypes.ObjectRef{Name: genericPublisher, Namespace: ns}
		}
		// expectDeleting asserts Ready names the deletion and the drain phase it
		// waits on; the common controller's own NotReady write would keep an
		// earlier False reason and message.
		expectDeleting := func(l *spectrev1.Listener, phase string) {
			ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
			Expect(ready).ToNot(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal("Deleting"))
			Expect(ready.Message).To(Equal("Draining capture before deletion (phase " + phase + ")"))
		}

		It("should checkpoint first, drain the RouteListener, then the Subscribers, then the Publisher, and release only when complete", func() {
			listener := newListener()
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}
			listener.Status.EventSubscriptions = []ctypes.ObjectRef{
				{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-rq"},
				{Name: "sub-rp", Namespace: listenerZoneStatus, UID: "uid-rp"},
			}
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
				Fingerprint: "fp-1",
				Publisher:   genericIn(listenerZoneStatus),
			}

			// D1: checkpoint only.
			mockDrainLists(
				[]gatewayv1.RouteListener{liveRL(listenerZoneStatus, "rl-a", "uid-a")},
				[]pubsubv1.Subscriber{liveSub(listenerZoneStatus, "sub-rq", "uid-rq"), liveSub(listenerZoneStatus, "sub-rp", "uid-rp")},
			)
			expectDrainPending(h.Delete(ctx, listener))
			d := listener.Status.Draining
			Expect(d).ToNot(BeNil())
			Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			Expect(d.Reason).To(Equal("listener deleted"))
			Expect(d.OldFingerprint).To(Equal("fp-1"))
			Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"}}))
			Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{
				{Name: "sub-rp", Namespace: listenerZoneStatus, UID: "uid-rp"},
				{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-rq"},
			}))
			Expect(d.SourcePublisher).To(Equal(genericIn(listenerZoneStatus)))
			expectDeleting(listener, handler.ExportDrainPhaseStopping)
			expectPassDone(0)

			// D2: the RouteListener is deleted with its preconditions.
			listener = persistListener(listener)
			expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", false)
			expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "11", nil)
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			expectPassDone(1)

			// D3: the RouteListener is gone; no Subscriber was touched yet.
			listener = persistListener(listener)
			expectGone(rlKind, listenerZoneStatus, "rl-a")
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
			Expect(listener.Status.RouteListener).To(BeNil())
			expectDeleting(listener, handler.ExportDrainPhaseDrainingSubscribers)
			expectPassDone(1)

			// D4: both Subscribers are deleted with their preconditions.
			listener = persistListener(listener)
			expectLive(subKind, listenerZoneStatus, "sub-rp", "uid-rp", "31", false)
			expectDelete(subKind, listenerZoneStatus, "sub-rp", "uid-rp", "31", nil)
			expectLive(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "32", false)
			expectDelete(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "32", nil)
			expectDrainPending(h.Delete(ctx, listener))
			expectPassDone(3)

			// D5: sub-rq still finalizes; the drain waits and keeps the Publisher.
			listener = persistListener(listener)
			expectGone(subKind, listenerZoneStatus, "sub-rp")
			expectLive(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "33", true)
			expectDelete(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "33", nil)
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
			expectPassDone(4)

			// D6: both Subscribers are gone.
			listener = persistListener(listener)
			expectGone(subKind, listenerZoneStatus, "sub-rp")
			expectGone(subKind, listenerZoneStatus, "sub-rq")
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			expectDeleting(listener, handler.ExportDrainPhaseCleaningPublisher)
			expectPassDone(4)

			// D7: the orphaned Publisher is deleted, a fresh inventory finds
			// nothing, and only now may the finalizer go.
			listener = persistListener(listener)
			expectOrphanCheck(listenerZoneStatus, nil)
			expectPublisherDelete(listenerZoneStatus)
			mockDrainLists(nil, nil)
			Expect(h.Delete(ctx, listener)).To(Succeed())
			Expect(listener.Status.Draining).To(BeNil())
			expectPassDone(5)
		})

		It("should continue a drain already in progress without re-snapshotting it", func() {
			listener := newListener()
			listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{Fingerprint: "fp-old"}
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:           handler.ExportDrainPhaseDrainingSubscribers,
				Reason:          "fingerprint changed",
				OldFingerprint:  "fp-old",
				OldSubscribers:  []ctypes.ObjectRef{{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-rq"}},
				SourcePublisher: genericIn(listenerZoneStatus),
			}
			// Ready was already False before the deletion.
			listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, "Approval has been revoked"))

			// D1: the recorded phase advances; no inventory List, no new checkpoint.
			expectLive(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "41", false)
			expectDelete(subKind, listenerZoneStatus, "sub-rq", "uid-rq", "41", nil)
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Reason).To(Equal("fingerprint changed"))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
			expectDeleting(listener, handler.ExportDrainPhaseDrainingSubscribers)
			expectPassDone(1)

			// D2: gone.
			listener = persistListener(listener)
			expectGone(subKind, listenerZoneStatus, "sub-rq")
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			expectDeleting(listener, handler.ExportDrainPhaseCleaningPublisher)
			expectPassDone(1)

			// D3: another observer keeps the Publisher; the drain completes and
			// the drained fingerprint does not start another one.
			listener = persistListener(listener)
			expectOrphanCheck(listenerZoneStatus, []pubsubv1.Subscriber{bridgeSub(listenerZoneStatus, "other-sub", false)})
			mockDrainLists(nil, nil)
			Expect(h.Delete(ctx, listener)).To(Succeed())
			Expect(listener.Status.Draining).To(BeNil())
			expectPassDone(1)
		})

		It("should not delete a same-name replacement, and drain it through a new checkpoint when it is this Listener's", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:           handler.ExportDrainPhaseDrainingSubscribers,
				OldSubscribers:  []ctypes.ObjectRef{{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-rq"}},
				SourcePublisher: genericIn(listenerZoneStatus),
			}

			// D1: the live sub-rq has another UID: the recorded instance is gone.
			expectLive(subKind, listenerZoneStatus, "sub-rq", "uid-new", "51", false)
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			expectPassDone(0)

			// D2: the replacement references the Publisher, which is kept. The
			// drain completes, but the fresh inventory finds the replacement owned
			// by this Listener: a new checkpoint, not a released finalizer.
			listener = persistListener(listener)
			replacement := bridgeSub(listenerZoneStatus, "sub-rq", false)
			replacement.UID = "uid-new"
			expectOrphanCheck(listenerZoneStatus, []pubsubv1.Subscriber{replacement})
			mockDrainLists(nil, []pubsubv1.Subscriber{replacement})
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			Expect(listener.Status.Draining.OldSubscribers).To(Equal([]ctypes.ObjectRef{
				{Name: "sub-rq", Namespace: listenerZoneStatus, UID: "uid-new"},
			}))
			expectPassDone(0)
		})

		It("should drain children in two namespaces with empty status refs instead of failing before deleting anything", func() {
			listener := newListener()

			// D1: nothing in status, owner-labelled children in two zones. The
			// checkpoint covers both; nothing is deleted yet.
			rlA := liveRL(listenerZoneStatus, "rl-a", "uid-a")
			rlB := liveRL(drainZoneB, "rl-b", "uid-b")
			subA := bridgeSub(listenerZoneStatus, "sub-a", false)
			subA.UID = "uid-sa"
			subB := bridgeSub(drainZoneB, "sub-b", false)
			subB.UID = "uid-sb"
			mockDrainLists([]gatewayv1.RouteListener{rlB, rlA}, []pubsubv1.Subscriber{subB, subA})
			expectDrainPending(h.Delete(ctx, listener))
			d := listener.Status.Draining
			Expect(d).ToNot(BeNil())
			Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{
				{Name: "rl-a", Namespace: listenerZoneStatus, UID: "uid-a"},
				{Name: "rl-b", Namespace: drainZoneB, UID: "uid-b"},
			}))
			Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{
				{Name: "sub-a", Namespace: listenerZoneStatus, UID: "uid-sa"},
				{Name: "sub-b", Namespace: drainZoneB, UID: "uid-sb"},
			}))
			Expect(d.SourcePublisher).To(BeNil()) // the bridges disagree
			expectPassDone(0)

			// D2: both RouteListeners are deleted.
			listener = persistListener(listener)
			expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "61", false)
			expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "61", nil)
			expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "62", false)
			expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "62", nil)
			expectDrainPending(h.Delete(ctx, listener))
			expectPassDone(2)

			// D3: both gone.
			listener = persistListener(listener)
			expectGone(rlKind, listenerZoneStatus, "rl-a")
			expectGone(rlKind, drainZoneB, "rl-b")
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
			expectPassDone(2)

			// D4: both Subscribers are deleted.
			listener = persistListener(listener)
			expectLive(subKind, listenerZoneStatus, "sub-a", "uid-sa", "63", false)
			expectDelete(subKind, listenerZoneStatus, "sub-a", "uid-sa", "63", nil)
			expectLive(subKind, drainZoneB, "sub-b", "uid-sb", "64", false)
			expectDelete(subKind, drainZoneB, "sub-b", "uid-sb", "64", nil)
			expectDrainPending(h.Delete(ctx, listener))
			expectPassDone(4)

			// D5: both gone.
			listener = persistListener(listener)
			expectGone(subKind, listenerZoneStatus, "sub-a")
			expectGone(subKind, drainZoneB, "sub-b")
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
			expectPassDone(4)

			// D6: the Publisher is cleaned in both namespaces, then released.
			listener = persistListener(listener)
			expectOrphanCheck(listenerZoneStatus, nil)
			expectPublisherDelete(listenerZoneStatus)
			expectOrphanCheck(drainZoneB, nil)
			expectPublisherDelete(drainZoneB)
			mockDrainLists(nil, nil)
			Expect(h.Delete(ctx, listener)).To(Succeed())
			Expect(listener.Status.Draining).To(BeNil())
			expectPassDone(6)
		})

		It("should keep the finalizer and the phase when a RouteListener delete fails", func() {
			listener := twoRouteListenerCheckpoint()
			expectLive(rlKind, listenerZoneStatus, "rl-a", "uid-a", "71", false)
			expectDelete(rlKind, listenerZoneStatus, "rl-a", "uid-a", "71", errors.NewServiceUnavailable("down"))
			expectLive(rlKind, drainZoneB, "rl-b", "uid-b", "72", false)
			expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "72", nil)
			err := h.Delete(ctx, listener)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("down"))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			expectPassDone(2)
		})

		It("should decide gone and take delete preconditions from the live read, not the cache", func() {
			const liveEnv = "test-env"
			ctx = contextutil.WithEnv(ctx, liveEnv)
			inEnv := map[string]string{cconfig.EnvironmentLabelKey: liveEnv}
			listener := twoRouteListenerCheckpoint()

			// The cache lags: it still shows rl-a (deleted on the server) and an
			// older rl-b. The Reader sees rl-a gone and rl-b at RV 91.
			for _, rl := range []gatewayv1.RouteListener{
				liveRL(listenerZoneStatus, "rl-a", "uid-a"), liveRL(drainZoneB, "rl-b", "uid-b"),
			} {
				stale := rl
				stale.ResourceVersion = "5"
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: rl.Name, Namespace: rl.Namespace}, mock.AnythingOfType("*v1.RouteListener")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						stale.DeepCopyInto(out.(*gatewayv1.RouteListener))
					}).
					Return(nil).Maybe()
			}
			rlB := liveRL(drainZoneB, "rl-b", "uid-b")
			rlB.ResourceVersion = "91"
			rlB.Labels = inEnv
			h.Reader = crfake.NewClientBuilder().WithScheme(scheme).WithObjects(&rlB).Build()

			// D1: only rl-b is deleted, with the Reader's UID and RV.
			expectDelete(rlKind, drainZoneB, "rl-b", "uid-b", "91", nil)
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			expectPassDone(1)

			// D2: the Reader sees both gone; the drain advances although the
			// cache still shows them.
			listener = persistListener(listener)
			h.Reader = crfake.NewClientBuilder().WithScheme(scheme).Build()
			expectDrainPending(h.Delete(ctx, listener))
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
			expectPassDone(1)
			fakeClient.AssertNumberOfCalls(GinkgoT(), "Get", 0)
		})

		It("should release at once when nothing was tracked, found or applied", func() {
			listener := newListener()
			mockDrainLists(nil, nil)
			Expect(h.Delete(ctx, listener)).To(Succeed())
			Expect(listener.Status.Draining).To(BeNil())
			expectPassDone(0)
		})
	})
})
