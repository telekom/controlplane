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
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// makeZone creates a Zone with a default gateway preset and gateway status reference.
func makeZone(name, statusNs string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: adminv1.ZoneSpec{
			Gateway: adminv1.GatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{{
					Name:    "default",
					Default: true,
					Urls: []adminv1.UrlConfig{{
						Hostname: "gateway.example.com",
						Port:     443,
						Scheme:   "https",
					}},
				}},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: statusNs,
			Gateway:   &ctypes.ObjectRef{Name: "gateway-" + name, Namespace: "default"},
		},
	}
}

// makeZoneNoPreset creates a Zone whose gateway config has no default preset.
func makeZoneNoPreset(name, statusNs string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: adminv1.ZoneSpec{
			Gateway: adminv1.GatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{{
					Name:    "non-default",
					Default: false,
					Urls: []adminv1.UrlConfig{{
						Hostname: "gateway.example.com",
						Port:     443,
						Scheme:   "https",
					}},
				}},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: statusNs,
			Gateway:   &ctypes.ObjectRef{Name: "gateway-" + name, Namespace: "default"},
		},
	}
}

// makeZoneNoGateway creates a Zone with a default preset but no gateway status reference.
func makeZoneNoGateway(name, statusNs string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: adminv1.ZoneSpec{
			Gateway: adminv1.GatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{{
					Name:    "default",
					Default: true,
					Urls: []adminv1.UrlConfig{{
						Hostname: "gateway.example.com",
						Port:     443,
						Scheme:   "https",
					}},
				}},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: statusNs,
			Gateway:   nil,
		},
	}
}

// ---------- CreateCallbackRoute ----------

var _ = Describe("CreateCallbackRoute", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		zone       *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		zone = makeZone("zone-a", "zone-a-ns")
	})

	It("should return BlockedError when zone has no default preset", func() {
		zoneNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreateCallbackRoute(ctx, zoneNoPreset)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when zone has no gateway reference in status", func() {
		zoneNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateCallbackRoute(ctx, zoneNoGw)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should create callback route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateCallbackRoute(ctx, zone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("callback--zone-a"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "callback"))

		// Verify upstream: localhost:8080/proxy
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("localhost"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/proxy"))

		// Verify hostnames and paths from preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(1))
		Expect(route.Spec.Paths[0]).To(Equal("/zone-a/callback/v1"))

		// Verify DynamicUpstream
		Expect(route.Spec.Traffic.DynamicUpstream).ToNot(BeNil())
		Expect(route.Spec.Traffic.DynamicUpstream.QueryParameter).To(Equal("callback"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeFalse())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())

		// Verify GatewayRef
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))
	})

	It("should add util.MeshClientName to DefaultConsumers when IsProxyTarget", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateCallbackRoute(ctx, zone, util.WithProxyTarget(true))
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(ContainElement("eventstore"))
	})

	It("should return error when CreateOrUpdate fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateCallbackRoute(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update callback Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateProxyCallbackRoute ----------

var _ = Describe("CreateProxyCallbackRoute", func() {
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

		route, err := util.CreateProxyCallbackRoute(ctx, sourceNoPreset, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when source zone has no gateway reference", func() {
		sourceNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreateProxyCallbackRoute(ctx, sourceNoGw, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return BlockedError when target zone has no default preset", func() {
		targetNoPreset := makeZoneNoPreset("zone-b", "zone-b-ns")

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetNoPreset)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should create proxy callback route successfully", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("callback--zone-b"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels reference source zone
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "callback-proxy"))

		// Verify upstream: from target zone's preset URL + callback path
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("gateway.example.com"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(443)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/zone-b/callback/v1"))

		// Verify hostnames and paths from source zone's preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(1))
		Expect(route.Spec.Paths[0]).To(Equal("/zone-b/callback/v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeFalse())

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

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update proxy callback Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateCallbackProxyRoutes ----------

var _ = Describe("CreateCallbackProxyRoutes", func() {
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

		routes, err := util.CreateCallbackProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
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

		routes, err := util.CreateCallbackProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
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

		routes, err := util.CreateCallbackProxyRoutes(ctx, &meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(HaveLen(2))
		Expect(routes).To(HaveKey("zone-b"))
		Expect(routes).To(HaveKey("zone-c"))
	})
})
