// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("Zone-health routes", func() {
	var zone *adminv1.Zone
	var handler *ZoneHandler
	var zoneIdx int

	BeforeEach(func() {
		zoneIdx++
		zone = newTestZone(fmt.Sprintf("zone-health-%d", zoneIdx))
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
		handler = newTestHandler()
	})
	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
	})

	reconcile := func() error {
		if err := handler.CreateOrUpdate(newTestContext(zone), zone); err != nil {
			return err
		}
		markSubResourcesReady(zone)
		return handler.CreateOrUpdate(newTestContext(zone), zone)
	}
	getRoute := func(gateway string) *gatewayapi.Route {
		route := &gatewayapi.Route{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: zone.Status.Namespace,
			Name:      naming.ForGateway(zone, gateway) + "--" + zoneHealthRouteSuffix,
		}, route)).To(Succeed())
		return route
	}

	DescribeTable("creates the reserved probe without DTC or managed routes",
		func(visibility adminv1.ZoneVisibility) {
			zone.Spec.Visibility = visibility
			Expect(reconcile()).To(Succeed())
			route := getRoute("standard")
			Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
			Expect(route.Spec.PassThrough).To(BeTrue())
			Expect(route.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "standard")))
			Expect(route.Spec.Hostnames).To(Equal([]string{"test-stargate.de"}))
			Expect(route.Spec.Paths).To(Equal([]string{zoneHealthPath}))
			Expect(route.Spec.Backend.Upstreams).To(Equal([]gatewayapi.Upstream{{
				Scheme: "http", Hostname: "localhost", Port: jumperIdentityPort, Path: gatewayapi.ZoneHealthUpstreamPath,
			}}))
			Expect(route.Spec.Security.RealmName).To(Equal(zone.Status.RealmName))
			Expect(route.Labels).To(HaveKeyWithValue(config.OwnerUidLabelKey, string(zone.UID)))
			Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, domainName))
		},
		Entry("world", adminv1.ZoneVisibilityWorld),
		Entry("enterprise", adminv1.ZoneVisibilityEnterprise),
	)

	It("is idempotent", func() {
		Expect(reconcile()).To(Succeed())
		before := getRoute("standard")
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		after := getRoute("standard")
		Expect(after.Spec).To(Equal(before.Spec))
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
	})

	It("covers sorted, deduplicated hostnames and joined base paths including DTC presets", func() {
		zone.Spec.Presets[0].Urls = []adminv1.UrlConfig{
			{Hostname: "z.example.com", BasePath: "/v1"},
			{Hostname: "a.example.com", BasePath: "/v1/"},
		}
		zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
			Name: "failover", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary",
			Urls:     []adminv1.UrlConfig{{Hostname: "a.example.com", BasePath: "/v2"}},
			Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}},
		})
		Expect(reconcile()).To(Succeed())
		route := getRoute("standard")
		Expect(route.Spec.Hostnames).To(Equal([]string{"a.example.com", "z.example.com"}))
		Expect(route.Spec.Paths).To(Equal([]string{"/v1/zone-health", "/v2/zone-health"}))
	})

	DescribeTable("creates and cleans up a route for every gateway type",
		func(gatewayType adminv1.GatewayType) {
			zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
				Name: "extra", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://extra.example.com/admin-api"},
			})
			zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
				Name: "extra", Type: gatewayType, Default: true, GatewayRef: "extra", IdentityProviderRef: "primary",
				Urls: []adminv1.UrlConfig{{Hostname: "extra.example.com", BasePath: "/"}},
			})
			Expect(reconcile()).To(Succeed())
			extra := getRoute("extra")
			Expect(extra.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "extra")))
			zone.Spec.Gateways = zone.Spec.Gateways[:1]
			zone.Spec.Presets = zone.Spec.Presets[:1]
			Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(extra), &gatewayapi.Route{})).To(MatchError(ContainSubstring("not found")))
			getRoute("standard")
		},
		Entry("AI", adminv1.GatewayTypeAI),
		Entry("Event", adminv1.GatewayTypeEvent),
	)

	It("keeps health routes when managed routes are removed", func() {
		zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{{
			Name: "proxy", Path: "/proxy", Url: "https://backend.example.com", Type: adminv1.ManagedRouteTypeProxy,
		}}}
		Expect(reconcile()).To(Succeed())
		before := getRoute("standard")
		zone.Spec.ManagedRoutes = nil
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(getRoute("standard").UID).To(Equal(before.UID))
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: zone.Status.Namespace, Name: naming.ForGateway(zone, "standard") + "--proxy",
		}, &gatewayapi.Route{})).To(MatchError(ContainSubstring("not found")))
	})

	DescribeTable("blocks legacy managed-route collisions without overwriting the Route or status",
		func(routeType adminv1.ManagedRouteType) {
			Expect(reconcile()).To(Succeed())
			existing := getRoute("standard")
			existing.Spec.Paths = []string{"/legacy"}
			existing.Spec.Backend.Upstreams[0].Hostname = "backend.example.com"
			existing.Spec.Backend.Upstreams[0].Path = "/legacy"
			Expect(k8sClient.Update(ctx, existing)).To(Succeed())
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{{
				Name: adminv1.ZoneHealthRouteName, Path: "/legacy", Url: "http://backend.example.com/legacy", Type: routeType,
			}}}
			zone.Status.ManagedRoutes = []types.ObjectRef{{Name: existing.Name, Namespace: existing.Namespace}}
			beforeStatus := append([]types.ObjectRef(nil), zone.Status.ManagedRoutes...)
			err := handler.CreateOrUpdate(newTestContext(zone), zone)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(blocked.IsBlocked()).To(BeTrue())
			Expect(err).To(MatchError(ContainSubstring("reserved for the platform health probe")))
			after := getRoute("standard")
			Expect(after.Spec).To(Equal(existing.Spec))
			Expect(after.ResourceVersion).To(Equal(existing.ResourceVersion))
			Expect(zone.Status.ManagedRoutes).To(Equal(beforeStatus))
		},
		Entry("Proxy", adminv1.ManagedRouteTypeProxy),
		Entry("TeamAPI", adminv1.ManagedRouteTypeTeamAPI),
	)

	DescribeTable("blocks excessive inputs before creating identity routes",
		func(hostnames bool) {
			count := maxRoutePaths + 1
			if hostnames {
				count = maxRouteHostnames + 1
			}
			zone.Spec.Presets = nil
			for i := 0; i < count; i++ {
				url := adminv1.UrlConfig{Hostname: "test-stargate.de", BasePath: fmt.Sprintf("/v%d", i)}
				if hostnames {
					url.Hostname = fmt.Sprintf("host%d.example.com", i)
					url.BasePath = "/"
				}
				if i%5 == 0 {
					zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
						Name: fmt.Sprintf("preset-%d", i), Type: adminv1.GatewayTypeAPI, Default: i == 0,
						GatewayRef: "standard", IdentityProviderRef: "primary",
					})
				}
				preset := &zone.Spec.Presets[len(zone.Spec.Presets)-1]
				preset.Urls = append(preset.Urls, url)
			}
			err := reconcile()
			Expect(err).To(HaveOccurred())
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(blocked.IsBlocked()).To(BeTrue())
			if hostnames {
				Expect(err).To(MatchError(ContainSubstring("hostnames")))
			} else {
				Expect(err).To(MatchError(ContainSubstring("paths")))
			}
			routes := &gatewayapi.RouteList{}
			Expect(k8sClient.List(ctx, routes, client.InNamespace(zone.Status.Namespace))).To(Succeed())
			Expect(routes.Items).To(BeEmpty())
		},
		Entry("hostnames", true),
		Entry("paths", false),
	)

	It("counts joined paths rather than raw base paths at the limit", func() {
		zone.Spec.Presets = nil
		for i := 0; i < maxRoutePaths; i++ {
			zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
				Name: fmt.Sprintf("preset-%d", i), Type: adminv1.GatewayTypeAPI, Default: i == 0,
				GatewayRef: "standard", IdentityProviderRef: "primary",
				Urls: []adminv1.UrlConfig{
					{Hostname: fmt.Sprintf("host%d.example.com", i), BasePath: fmt.Sprintf("/v%d", i)},
					{Hostname: fmt.Sprintf("host%d.example.com", i+maxRoutePaths), BasePath: fmt.Sprintf("/v%d/", i)},
				},
			})
		}
		Expect(reconcile()).To(Succeed())
		Expect(getRoute("standard").Spec.Paths).To(HaveLen(maxRoutePaths))
		Expect(getRoute("standard").Spec.Hostnames).To(HaveLen(maxRouteHostnames))
	})
})
