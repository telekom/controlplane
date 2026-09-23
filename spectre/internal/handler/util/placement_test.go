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

	Context("same-zone (A==C)", func() {
		It("should resolve placement when capture and observer are the same zone", func() {
			zone := makeZone("consumer-zone")
			ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
			ec.Status.CallbackURL = "https://gateway.example.com/horizon/callback/v1"
			esRef := &ctypes.ObjectRef{Name: "es-consumer", Namespace: "test-env--consumer-zone"}
			ec.Status.EventStore = esRef
			es := makeReadyEventStore("es-consumer", "test-env--consumer-zone")

			// GetEventConfig: List EventConfigs for capture zone.
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

			placement, err := util.ResolvePlacement(ctx, zone, zone, route)
			Expect(err).ToNot(HaveOccurred())
			Expect(placement).ToNot(BeNil())

			// Same-zone: capture == delivery, local callback.
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

		It("should work with the provider zone as both capture and observer (fallback case)", func() {
			zone := makeZone("provider-zone")
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

			placement, err := util.ResolvePlacement(ctx, zone, zone, route)
			Expect(err).ToNot(HaveOccurred())
			Expect(placement).ToNot(BeNil())
			Expect(placement.CaptureZone.Name).To(Equal("provider-zone"))
			Expect(placement.DeliveryZone.Name).To(Equal("provider-zone"))
			Expect(placement.BridgeNamespace).To(Equal("test-env--provider-zone"))
			Expect(placement.CallbackBaseURL).To(Equal("https://gateway.provider.example.com/horizon/callback/v1"))
		})

		It("should return BlockedError when same-zone EventConfig has no CallbackURL", func() {
			zone := makeZone("consumer-zone")
			ec := makeReadyEventConfig("ec-consumer", "consumer-zone")
			ec.Status.CallbackURL = "" // Empty CallbackURL
			esRef := &ctypes.ObjectRef{Name: "es-consumer", Namespace: "test-env--consumer-zone"}
			ec.Status.EventStore = esRef
			es := makeReadyEventStore("es-consumer", "test-env--consumer-zone")

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

			placement, err := util.ResolvePlacement(ctx, zone, zone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("CallbackURL"))
		})

		It("should set BridgeNamespace from EventStore namespace", func() {
			zone := makeZone("consumer-zone")
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

			placement, err := util.ResolvePlacement(ctx, zone, zone, route)
			Expect(err).ToNot(HaveOccurred())
			Expect(placement.BridgeNamespace).To(Equal("custom-namespace"))
		})
	})

	Context("capture-zone errors (common to same-zone and cross-zone)", func() {
		It("should return BlockedError when capture zone has no EventConfig", func() {
			captureZone := makeZone("empty-zone")
			observerZone := makeZone("empty-zone")

			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{}}
				}).
				Return(nil).
				Once()

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("EventConfig"))
		})

		It("should return BlockedError when capture EventConfig has no EventStore reference", func() {
			zone := makeZone("consumer-zone")
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

			placement, err := util.ResolvePlacement(ctx, zone, zone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("EventStore"))
		})

		It("should propagate API errors from GetEventConfig", func() {
			captureZone := makeZone("consumer-zone")
			observerZone := makeZone("consumer-zone")

			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Return(fmt.Errorf("connection refused")).
				Once()

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
	})

	Context("cross-zone (capture != observer)", func() {
		It("should resolve separate capture and delivery infrastructure with proxy callback", func() {
			captureZone := makeZone("capture-zone")
			observerZone := makeZone("observer-zone")

			captureEC := makeReadyEventConfig("ec-capture", "capture-zone")
			captureEC.Status.CallbackURL = "https://gateway.capture.example.com/horizon/callback/v1"
			captureESRef := &ctypes.ObjectRef{Name: "es-capture", Namespace: "test-env--capture-zone"}
			captureEC.Status.EventStore = captureESRef
			captureES := makeReadyEventStore("es-capture", "test-env--capture-zone")

			deliveryEC := makeReadyEventConfig("ec-observer", "observer-zone")
			deliveryEC.Status.CallbackURL = "https://gateway.observer.example.com/horizon/callback/v1"
			deliveryEC.Status.ProxyCallbackURLs = map[string]string{
				"capture-zone": "https://gateway.observer.example.com/horizon/proxy-callback/capture-zone/v1",
			}
			deliveryESRef := &ctypes.ObjectRef{Name: "es-observer", Namespace: "test-env--observer-zone"}
			deliveryEC.Status.EventStore = deliveryESRef
			deliveryES := makeReadyEventStore("es-observer", "test-env--observer-zone")

			// Call 1: GetEventConfig for capture zone.
			// Call 2: GetEventConfig for observer/delivery zone.
			listResponder := sequentialListResponder(
				[]eventv1.EventConfig{captureEC},
				[]eventv1.EventConfig{deliveryEC},
			)
			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Run(listResponder).
				Return(nil).
				Times(2)

			// Call 1: ResolveEventStore for capture EventConfig.
			// Call 2: ResolveEventStore for delivery EventConfig.
			getCallCount := 0
			fakeClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
					getCallCount++
					if getCallCount == 1 {
						*obj.(*pubsubv1.EventStore) = *captureES
					} else {
						*obj.(*pubsubv1.EventStore) = *deliveryES
					}
				}).
				Return(nil).
				Times(2)

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).ToNot(HaveOccurred())
			Expect(placement).ToNot(BeNil())

			// Capture side.
			Expect(placement.CaptureZone.Name).To(Equal("capture-zone"))
			Expect(placement.CaptureEventConfig.Name).To(Equal("ec-capture"))
			Expect(placement.CaptureEventStore.Name).To(Equal("es-capture"))
			Expect(placement.CaptureRoute).To(Equal(route))

			// Delivery side.
			Expect(placement.DeliveryZone.Name).To(Equal("observer-zone"))
			Expect(placement.DeliveryEventConfig.Name).To(Equal("ec-observer"))
			Expect(placement.DeliveryEventStore.Name).To(Equal("es-observer"))

			// Callback origin is always the capture zone.
			Expect(placement.CallbackOriginZone.Name).To(Equal("capture-zone"))
			// Proxy callback URL from delivery EventConfig.
			Expect(placement.CallbackBaseURL).To(Equal("https://gateway.observer.example.com/horizon/proxy-callback/capture-zone/v1"))
			// Bridge namespace from capture EventStore.
			Expect(placement.BridgeNamespace).To(Equal("test-env--capture-zone"))
		})

		It("should return BlockedError when observer zone has no EventConfig", func() {
			captureZone := makeZone("capture-zone")
			observerZone := makeZone("observer-zone")

			captureEC := makeReadyEventConfig("ec-capture", "capture-zone")
			captureEC.Status.CallbackURL = "https://gateway.capture.example.com/horizon/callback/v1"
			captureESRef := &ctypes.ObjectRef{Name: "es-capture", Namespace: "test-env--capture-zone"}
			captureEC.Status.EventStore = captureESRef
			captureES := makeReadyEventStore("es-capture", "test-env--capture-zone")

			// Call 1: capture zone has EventConfig.
			// Call 2: observer zone has no EventConfig.
			listResponder := sequentialListResponder(
				[]eventv1.EventConfig{captureEC},
				[]eventv1.EventConfig{}, // observer zone: no EventConfig
			)
			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Run(listResponder).
				Return(nil).
				Times(2)

			fakeClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
					*obj.(*pubsubv1.EventStore) = *captureES
				}).
				Return(nil).
				Once()

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("delivery EventConfig"))
		})

		It("should return BlockedError when delivery EventConfig has nil ProxyCallbackURLs", func() {
			captureZone := makeZone("capture-zone")
			observerZone := makeZone("observer-zone")

			captureEC := makeReadyEventConfig("ec-capture", "capture-zone")
			captureESRef := &ctypes.ObjectRef{Name: "es-capture", Namespace: "test-env--capture-zone"}
			captureEC.Status.EventStore = captureESRef
			captureES := makeReadyEventStore("es-capture", "test-env--capture-zone")

			deliveryEC := makeReadyEventConfig("ec-observer", "observer-zone")
			deliveryEC.Status.ProxyCallbackURLs = nil // No proxy callbacks configured
			deliveryESRef := &ctypes.ObjectRef{Name: "es-observer", Namespace: "test-env--observer-zone"}
			deliveryEC.Status.EventStore = deliveryESRef
			deliveryES := makeReadyEventStore("es-observer", "test-env--observer-zone")

			listResponder := sequentialListResponder(
				[]eventv1.EventConfig{captureEC},
				[]eventv1.EventConfig{deliveryEC},
			)
			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Run(listResponder).
				Return(nil).
				Times(2)

			getCallCount := 0
			fakeClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
					getCallCount++
					if getCallCount == 1 {
						*obj.(*pubsubv1.EventStore) = *captureES
					} else {
						*obj.(*pubsubv1.EventStore) = *deliveryES
					}
				}).
				Return(nil).
				Times(2)

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("ProxyCallbackURLs"))
		})

		It("should return BlockedError when ProxyCallbackURLs has no entry for capture zone", func() {
			captureZone := makeZone("capture-zone")
			observerZone := makeZone("observer-zone")

			captureEC := makeReadyEventConfig("ec-capture", "capture-zone")
			captureESRef := &ctypes.ObjectRef{Name: "es-capture", Namespace: "test-env--capture-zone"}
			captureEC.Status.EventStore = captureESRef
			captureES := makeReadyEventStore("es-capture", "test-env--capture-zone")

			deliveryEC := makeReadyEventConfig("ec-observer", "observer-zone")
			deliveryEC.Status.ProxyCallbackURLs = map[string]string{
				"other-zone": "https://gateway.observer.example.com/horizon/proxy-callback/other-zone/v1",
			} // No entry for "capture-zone"
			deliveryESRef := &ctypes.ObjectRef{Name: "es-observer", Namespace: "test-env--observer-zone"}
			deliveryEC.Status.EventStore = deliveryESRef
			deliveryES := makeReadyEventStore("es-observer", "test-env--observer-zone")

			listResponder := sequentialListResponder(
				[]eventv1.EventConfig{captureEC},
				[]eventv1.EventConfig{deliveryEC},
			)
			fakeClient.EXPECT().
				List(mock.Anything, mock.Anything, mock.Anything).
				Run(listResponder).
				Return(nil).
				Times(2)

			getCallCount := 0
			fakeClient.EXPECT().
				Get(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
					getCallCount++
					if getCallCount == 1 {
						*obj.(*pubsubv1.EventStore) = *captureES
					} else {
						*obj.(*pubsubv1.EventStore) = *deliveryES
					}
				}).
				Return(nil).
				Times(2)

			placement, err := util.ResolvePlacement(ctx, captureZone, observerZone, route)
			Expect(err).To(HaveOccurred())
			Expect(placement).To(BeNil())
			Expect(err).To(Satisfy(isBlockedError))
			Expect(err.Error()).To(ContainSubstring("proxy callback"))
			Expect(err.Error()).To(ContainSubstring("capture-zone"))
		})
	})
})
