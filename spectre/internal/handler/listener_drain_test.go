// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
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
		It("should record drain status with old children refs", func() {
			listener := newListener()
			listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
			listener.Status.EventSubscriptions = []ctypes.ObjectRef{
				{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				{Name: "old-sub-rp", Namespace: listenerZoneStatus},
			}

			err := h.StartDrain(ctx, listener, "fingerprint changed", "old-fp-abc")
			Expect(err).ToNot(HaveOccurred())

			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			Expect(listener.Status.Draining.Reason).To(Equal("fingerprint changed"))
			Expect(listener.Status.Draining.OldFingerprint).To(Equal("old-fp-abc"))
			Expect(listener.Status.Draining.OldRouteListener).ToNot(BeNil())
			Expect(listener.Status.Draining.OldRouteListener.Name).To(Equal("old-rl"))
			Expect(listener.Status.Draining.OldSubscribers).To(HaveLen(2))
		})

		It("should handle nil status refs gracefully", func() {
			listener := newListener()

			err := h.StartDrain(ctx, listener, "test", "old-fp")
			Expect(err).ToNot(HaveOccurred())

			Expect(listener.Status.Draining).ToNot(BeNil())
			Expect(listener.Status.Draining.OldRouteListener).To(BeNil())
			Expect(listener.Status.Draining.OldSubscribers).To(BeNil())
		})
	})

	Describe("ContinueDrain", func() {
		It("should return true when draining is nil", func() {
			listener := newListener()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
		})

		It("should complete drain when old children are gone", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				Reason:         "fingerprint changed",
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
				},
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				},
			}

			// Old RouteListener is gone.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Return(errors.NewNotFound(
					schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "routelisteners"}, "old-rl")).
				Once()

			// Old Subscriber is gone.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-sub-rq", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.Subscriber")).
				Return(errors.NewNotFound(
					schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "subscribers"}, "old-sub-rq")).
				Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})

		It("should wait when old RouteListener still exists", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
				},
			}

			// Old RouteListener still exists.
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "old-rl", Namespace: listenerZoneStatus},
					mock.AnythingOfType("*v1.RouteListener")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					rl := out.(*gatewayv1.RouteListener)
					rl.Name = "old-rl"
					rl.Namespace = listenerZoneStatus
				}).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
			Expect(listener.Status.Draining).ToNot(BeNil())
		})

		It("should wait when old Subscriber still exists", func() {
			listener := newListener()
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseDrainingSubscribers,
				OldFingerprint: "old-fp",
				OldSubscribers: []ctypes.ObjectRef{
					{Name: "old-sub", Namespace: listenerZoneStatus},
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
				}).
				Return(nil).Once()

			done, err := h.ContinueDrain(ctx, listener)
			Expect(err).ToNot(HaveOccurred())
			Expect(done).To(BeFalse())
		})

		It("should complete from DrainingSubscribers when all old Subscribers gone", func() {
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

		It("should survive restart: drain status persists and resumes", func() {
			listener := newListener()
			// Simulate: controller restarted while Stopping.
			listener.Status.Draining = &spectrev1.ListenerDrainStatus{
				Phase:          handler.ExportDrainPhaseStopping,
				Reason:         "fingerprint changed",
				OldFingerprint: "old-fp",
				OldRouteListener: &ctypes.ObjectRef{
					Name:      "old-rl",
					Namespace: listenerZoneStatus,
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
			Expect(done).To(BeTrue())
			Expect(listener.Status.Draining).To(BeNil())
		})
	})
})
