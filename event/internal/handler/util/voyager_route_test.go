// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
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

// ---------- CreateVoyagerRoute ----------

var _ = Describe("CreateVoyagerRoute", func() {
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
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-zone-a",
				Namespace: "zone-a-ns",
				UID:       k8stypes.UID("ec-uid-1234"),
			},
			Spec: eventv1.EventConfigSpec{
				Local: &eventv1.LocalBackend{VoyagerApiUrl: "http://voyager-service:8080/voyager"},
			},
		}
	})

	It("should return BlockedError when zone has no default preset", func() {
		zoneNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreateVoyagerRoute(ctx, zoneNoPreset, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when zone has no gateway reference in status", func() {
		zoneNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateVoyagerRoute(ctx, zoneNoGw, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return error when voyagerApiUrl is invalid", func() {
		badConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-bad", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Local: &eventv1.LocalBackend{VoyagerApiUrl: "://bad-url"},
			},
		}

		route, err := util.CreateVoyagerRoute(ctx, zone, badConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to parse voyagerApiUrl"))
	})

	It("should create voyager route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateVoyagerRoute(ctx, zone, eventConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("voyager--zone-a"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "voyager"))

		// Verify upstream: from voyagerApiUrl
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("voyager-service"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/voyager"))

		// Verify hostnames and paths from preset (voyager has 2 paths: mesh + local)
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(2))
		Expect(route.Spec.Paths[0]).To(Equal("/horizon-zone-a/voyager/v1"))
		Expect(route.Spec.Paths[1]).To(Equal("/horizon/voyager/v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())

		// Verify GatewayRef
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))
	})

	It("should add GatewayConsumerName to DefaultConsumers when IsProxyTarget", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateVoyagerRoute(ctx, zone, eventConfig, util.WithProxyTarget(true))
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(gatewayapi.GatewayConsumerName))
		Expect(route.Spec.Security.DefaultConsumers).ToNot(ContainElement(util.CallbackClientName))
	})

	It("should set owner reference when WithOwner is provided", func() {
		s := runtime.NewScheme()
		Expect(eventv1.AddToScheme(s)).To(Succeed())
		Expect(gatewayapi.AddToScheme(s)).To(Succeed())
		fakeClient.EXPECT().Scheme().Return(s).Maybe()

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateVoyagerRoute(ctx, zone, eventConfig, util.WithOwner(eventConfig))
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify owner reference was set
		Expect(route.GetOwnerReferences()).To(HaveLen(1))
		Expect(route.GetOwnerReferences()[0].Name).To(Equal("ec-zone-a"))
		Expect(route.GetOwnerReferences()[0].UID).To(Equal(k8stypes.UID("ec-uid-1234")))
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateVoyagerRoute(ctx, zone, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateProxyVoyagerRoute ----------

var _ = Describe("CreateProxyVoyagerRoute", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		sourceZone *adminv1.Zone
		targetZone *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		sourceZone = makeZone("zone-a", "zone-a-ns")
		targetZone = makeZone("zone-b", "zone-b-ns")
	})

	It("should return BlockedError when source zone has no default preset", func() {
		sourceNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreateProxyVoyagerRoute(ctx, sourceNoPreset, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when source zone has no gateway reference", func() {
		sourceNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateProxyVoyagerRoute(ctx, sourceNoGw, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return BlockedError when target zone has no default preset", func() {
		targetNoPreset := makeZoneNoPreset("zone-b", "zone-b-ns")

		route, err := util.CreateProxyVoyagerRoute(ctx, sourceZone, targetNoPreset)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should create proxy voyager route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateProxyVoyagerRoute(ctx, sourceZone, targetZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("voyager--zone-b"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels reference source zone
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "voyager-proxy"))

		// Verify upstream: from target zone's preset URL + voyager path
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("gateway.example.com"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(443)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/horizon-zone-b/voyager/v1"))

		// Verify hostnames and paths from source zone's preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(1))
		Expect(route.Spec.Paths[0]).To(Equal("/horizon-zone-b/voyager/v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify GatewayRef points to source zone's gateway
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))

		// Verify route type is proxy
		Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateProxyVoyagerRoute(ctx, sourceZone, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateProxyLocalVoyagerRoute ----------

var _ = Describe("CreateProxyLocalVoyagerRoute", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		sourceZone *adminv1.Zone
		targetZone *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		sourceZone = makeZone("zone-a", "zone-a-ns")
		targetZone = makeZone("zone-b", "zone-b-ns")
	})

	It("should return BlockedError when source zone has no default preset", func() {
		sourceNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreateProxyLocalVoyagerRoute(ctx, sourceNoPreset, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when source zone has no gateway reference", func() {
		sourceNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateProxyLocalVoyagerRoute(ctx, sourceNoGw, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return BlockedError when target zone has no default preset", func() {
		targetNoPreset := makeZoneNoPreset("zone-b", "zone-b-ns")

		route, err := util.CreateProxyLocalVoyagerRoute(ctx, sourceZone, targetNoPreset)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(unwrapAll(err)).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should create the own-zone proxy route with both mesh and local paths", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateProxyLocalVoyagerRoute(ctx, sourceZone, targetZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Name and namespace reference the source (proxy) zone
		Expect(route.Name).To(Equal("voyager--zone-a"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Labels: own voyager route (not the voyager-proxy mesh type)
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "voyager"))

		// Proxy route type, forwarding to the target zone's gateway voyager path
		Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("gateway.example.com"))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/horizon-zone-b/voyager/v1"))

		// Downstream serves both the mesh path (own zone) and the local shortcut
		Expect(route.Spec.Paths).To(HaveLen(2))
		Expect(route.Spec.Paths[0]).To(Equal("/horizon-zone-a/voyager/v1"))
		Expect(route.Spec.Paths[1]).To(Equal("/horizon/voyager/v1"))

		// Downstream is served by the source zone's gateway
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateProxyLocalVoyagerRoute(ctx, sourceZone, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Route"))
	})
})

// ---------- CreateVoyagerProxyRoutes ----------

var _ = Describe("CreateVoyagerProxyRoutes", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		sourceZone *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		sourceZone = makeZone("zone-a", "zone-a-ns")
	})

	It("should return empty map when no target zones after filtering", func() {
		meshConfig := eventv1.MeshConfig{
			FullMesh:  false,
			ZoneNames: []string{},
		}
		targetZones := []*adminv1.Zone{
			makeZone("zone-b", "zone-b-ns"),
		}

		routes, err := util.CreateVoyagerProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(BeEmpty())
	})

	It("should skip source zone in full mesh", func() {
		meshConfig := eventv1.MeshConfig{FullMesh: true}
		targetZoneB := makeZone("zone-b", "zone-b-ns")
		// Include source zone in targets to test skipping
		targetZones := []*adminv1.Zone{sourceZone, targetZoneB}

		// CreateOrUpdate for proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		routes, err := util.CreateVoyagerProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(HaveLen(1))
		Expect(routes).To(HaveKey("zone-b"))
	})

	It("should create routes for multiple target zones", func() {
		meshConfig := eventv1.MeshConfig{FullMesh: true}
		targetZoneB := makeZone("zone-b", "zone-b-ns")
		targetZoneC := makeZone("zone-c", "zone-c-ns")
		targetZones := []*adminv1.Zone{targetZoneB, targetZoneC}

		// CreateOrUpdate for zone-b proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		// CreateOrUpdate for zone-c proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		routes, err := util.CreateVoyagerProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(HaveLen(2))
		Expect(routes).To(HaveKey("zone-b"))
		Expect(routes).To(HaveKey("zone-c"))
	})
})
