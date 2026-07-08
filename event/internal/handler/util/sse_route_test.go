// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- CreateSSERoute ----------

var _ = Describe("CreateSSERoute", func() {
	var (
		ctx         context.Context
		fakeClient  *fakeclient.MockJanitorClient
		zone        *adminv1.Zone
		eventConfig *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		zone = makeZone("zone-a", "zone-a-ns")
		eventConfig = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				ServerSendEventUrl: "http://sse-service:8080/sse",
			},
		}
	})

	It("should return BlockedError when zone has no default preset", func() {
		zoneNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zoneNoPreset, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when zone has no gateway reference in status", func() {
		zoneNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zoneNoGw, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return error when ServerSendEventUrl is invalid", func() {
		badConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-bad", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				ServerSendEventUrl: "://bad-url",
			},
		}

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, badConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to parse ServerSendEventUrl"))
	})

	It("should create SSE route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("sse--de-telekom-test-v1"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "sse"))
		Expect(route.Labels).To(HaveKey(eventv1.EventTypeLabelKey))

		// Verify upstream: from ServerSendEventUrl
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("sse-service"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/sse"))

		// Verify hostnames and paths from preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(1))
		Expect(route.Spec.Paths[0]).To(Equal("/sse/v1/de.telekom.test.v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())

		// Verify Buffering: response buffering must be disabled for SSE streaming
		Expect(route.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
		Expect(route.Spec.Buffering.DisableRequestBuffering).To(BeFalse())

		// Verify GatewayRef
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))
	})

	It("should add MeshClientName to DefaultConsumers when isTargetOfProxy is true", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(util.MeshClientName))
	})

	It("should NOT add MeshClientName to DefaultConsumers when isTargetOfProxy is false", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update SSE Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CleanupOldSSERoutes ----------

var _ = Describe("CleanupOldSSERoutes", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return 0 deleted when Cleanup removes nothing", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, nil)

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(deleted).To(Equal(0))
	})

	It("should return count of deleted routes on success", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(3, nil)

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(deleted).To(Equal(3))
	})

	It("should return error when Cleanup fails", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, fmt.Errorf("cleanup failed"))

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(deleted).To(Equal(0))
		Expect(err.Error()).To(ContainSubstring("failed to cleanup old SSE Routes"))
		Expect(err.Error()).To(ContainSubstring("de.telekom.test.v1"))
	})

	It("should return partial count when Cleanup fails after some deletions", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(2, fmt.Errorf("partial cleanup failure"))

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(deleted).To(Equal(2))
		Expect(err.Error()).To(ContainSubstring("failed to cleanup old SSE Routes"))
	})
})

// ---------- CreateSSEProxyRoute ----------

var _ = Describe("CreateSSEProxyRoute", func() {
	var (
		ctx            context.Context
		fakeClient     *fakeclient.MockJanitorClient
		subscriberZone *adminv1.Zone
		providerZone   *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		subscriberZone = makeZone("zone-sub", "zone-sub-ns")
		providerZone = makeZone("zone-prov", "zone-prov-ns")
	})

	It("should return BlockedError when subscriber zone has no default preset", func() {
		subNoPreset := makeZoneNoPreset("zone-sub", "zone-sub-ns")

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", subNoPreset, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("subscriber zone"))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when subscriber zone has no gateway reference", func() {
		subNoGw := makeZoneNoGateway("zone-sub", "zone-sub-ns")

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", subNoGw, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("subscriber zone"))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return BlockedError when provider zone has no default preset", func() {
		provNoPreset := makeZoneNoPreset("zone-prov", "zone-prov-ns")

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", subscriberZone, provNoPreset)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("provider zone"))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should create SSE proxy route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", subscriberZone, providerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata: created in subscriber zone's namespace
		Expect(route.Name).To(Equal("sse--de-telekom-test-v1"))
		Expect(route.Namespace).To(Equal("zone-sub-ns"))

		// Verify labels reference subscriber zone
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-sub"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "sse-proxy"))
		Expect(route.Labels).To(HaveKey(eventv1.EventTypeLabelKey))

		// Verify upstream: from provider zone's preset URL + SSE path
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("gateway.example.com"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(443)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/sse/v1/de.telekom.test.v1"))

		// Verify hostnames and paths from subscriber zone's preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(1))
		Expect(route.Spec.Paths[0]).To(Equal("/sse/v1/de.telekom.test.v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify Buffering: response buffering must be disabled for SSE streaming
		Expect(route.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
		Expect(route.Spec.Buffering.DisableRequestBuffering).To(BeFalse())

		// Verify GatewayRef points to subscriber zone's gateway
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-sub"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))

		// Verify route type is proxy
		Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update SSE proxy Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})
