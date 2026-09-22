// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// makeRoute creates a minimal gateway Route for testing.
func makeRoute(name, namespace, path string) *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gatewayv1.RouteSpec{
			Paths: []string{path},
		},
	}
}

var _ = Describe("ResolvePlacement", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		route      *gatewayv1.Route
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		route = makeRoute("route-api-v1", "test-env--consumer-zone", "/api/v1")
	})

	It("should resolve placement for the listening zone (Phase 2: capture == delivery)", func() {
		listeningZone := makeZone("consumer-zone")
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
		ec.Status.CallbackURL = "https://gateway.example.com/horizon/callback/v1"
		esRef := &ctypes.ObjectRef{Name: "es-consumer", Namespace: "test-env--consumer-zone"}
		ec.Status.EventStore = esRef
		es := makeReadyEventStore("es-consumer", "test-env--consumer-zone")

		// GetEventConfig: List EventConfigs for listening zone.
		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		// ResolveEventStore: Get EventStore by ref.
		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).ToNot(HaveOccurred())
		Expect(placement).ToNot(BeNil())

		// Phase 2: capture == delivery == listening zone.
		Expect(placement.CaptureZone.Name).To(Equal("consumer-zone"))
		Expect(placement.DeliveryZone.Name).To(Equal("consumer-zone"))
		Expect(placement.CallbackOriginZone.Name).To(Equal("consumer-zone"))
		Expect(placement.CaptureRoute).To(Equal(route))
		Expect(placement.CaptureEventConfig.Name).To(Equal("ec-consumer"))
		Expect(placement.CaptureEventStore.Name).To(Equal("es-consumer"))
		Expect(placement.DeliveryEventConfig.Name).To(Equal("ec-consumer"))
		Expect(placement.DeliveryEventStore.Name).To(Equal("es-consumer"))
		Expect(placement.BridgeNamespace).To(Equal("test-env--consumer-zone"))
		Expect(placement.CallbackBaseURL).To(Equal("https://gateway.example.com/horizon/callback/v1"))
	})

	It("should work with the provider zone as listening zone (fallback case)", func() {
		listeningZone := makeZone("provider-zone")
		ec := makeReadyEventConfig("ec-provider", "provider-zone")
		ec.Status.CallbackURL = "https://gateway.provider.example.com/horizon/callback/v1"
		esRef := &ctypes.ObjectRef{Name: "es-provider", Namespace: "test-env--provider-zone"}
		ec.Status.EventStore = esRef
		es := makeReadyEventStore("es-provider", "test-env--provider-zone")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).ToNot(HaveOccurred())
		Expect(placement).ToNot(BeNil())
		Expect(placement.CaptureZone.Name).To(Equal("provider-zone"))
		Expect(placement.DeliveryZone.Name).To(Equal("provider-zone"))
		Expect(placement.BridgeNamespace).To(Equal("test-env--provider-zone"))
		Expect(placement.CallbackBaseURL).To(Equal("https://gateway.provider.example.com/horizon/callback/v1"))
	})

	It("should return BlockedError when listening zone has no EventConfig", func() {
		listeningZone := makeZone("empty-zone")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{}}
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).To(HaveOccurred())
		Expect(placement).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("EventConfig"))
	})

	It("should return BlockedError when EventConfig has no CallbackURL", func() {
		listeningZone := makeZone("consumer-zone")
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
		ec.Status.CallbackURL = "" // Empty CallbackURL

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).To(HaveOccurred())
		Expect(placement).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("CallbackURL"))
	})

	It("should return BlockedError when EventConfig has no EventStore reference", func() {
		listeningZone := makeZone("consumer-zone")
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
		ec.Status.CallbackURL = "https://gateway.example.com/horizon/callback/v1"
		// Status.EventStore is nil (default from makeReadyEventConfig).

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).To(HaveOccurred())
		Expect(placement).To(BeNil())
		Expect(err).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("EventStore"))
	})

	It("should propagate API errors from GetEventConfig", func() {
		listeningZone := makeZone("consumer-zone")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Return(fmt.Errorf("connection refused")).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).To(HaveOccurred())
		Expect(placement).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should set BridgeNamespace from EventStore namespace", func() {
		listeningZone := makeZone("consumer-zone")
		ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
		ec.Status.CallbackURL = "https://gateway.example.com/horizon/callback/v1"
		esRef := &ctypes.ObjectRef{Name: "es-custom-ns", Namespace: "custom-namespace"}
		ec.Status.EventStore = esRef
		es := makeReadyEventStore("es-custom-ns", "custom-namespace")

		fakeClient.EXPECT().
			List(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{ec}}
			}).
			Return(nil).
			Once()

		fakeClient.EXPECT().
			Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
				*obj.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).
			Once()

		placement, err := util.ResolvePlacement(ctx, listeningZone, route)
		Expect(err).ToNot(HaveOccurred())
		Expect(placement.BridgeNamespace).To(Equal("custom-namespace"))
	})
})
