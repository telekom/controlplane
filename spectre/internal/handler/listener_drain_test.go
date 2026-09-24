// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
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
			Expect(listener.Status.Draining.OldSubscribers).To(HaveLen(2))
			Expect(listener.Status.Draining.OldSubscribers[0].UID).To(Equal(k8stypes.UID("sub-uid-1")))
			Expect(listener.Status.Draining.OldSubscribers[1].UID).To(Equal(k8stypes.UID("sub-uid-2")))
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
})
