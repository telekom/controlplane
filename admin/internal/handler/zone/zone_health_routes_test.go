// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	config "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zone-health routes", func() {
	var zone *adminv1.Zone
	var zoneIdx int

	BeforeEach(func() {
		zoneIdx++
		zone = newTestZone(fmt.Sprintf("zone-health-%d", zoneIdx))
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
	})

	AfterEach(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })

	reconcile := func() {
		GinkgoHelper()
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
	}

	zoneHealthKey := func(gatewayName string) client.ObjectKey {
		return client.ObjectKey{
			Namespace: zone.Status.Namespace,
			Name:      naming.ForGateway(zone, gatewayName) + "--zone-health",
		}
	}

	getZoneHealthRoute := func(gatewayName string) *gatewayapi.Route {
		GinkgoHelper()
		route := &gatewayapi.Route{}
		Expect(k8sClient.Get(ctx, zoneHealthKey(gatewayName), route)).To(Succeed())
		return route
	}

	addAiGateway := func() {
		secret := "ai-secret"
		zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
			Name: "ai", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", ClientSecret: &secret, Url: "https://ai.example.com/admin-api"},
		})
		zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
			Name: "ai", Type: adminv1.GatewayTypeAI, Default: true, GatewayRef: "ai", IdentityProviderRef: "primary",
			Urls: []adminv1.UrlConfig{{Hostname: "ai.example.com", BasePath: "/"}},
		})
	}

	It("creates a HEAD-only route answered by the gateway without managed routes or DTC", func() {
		Expect(zone.Spec.ManagedRoutes).To(BeNil())
		Expect(zone.Spec.Presets[0].SupportsFeatures([]adminv1.FeatureName{adminv1.FeatureConsumerFailover})).To(BeFalse())

		reconcile()

		route := getZoneHealthRoute("standard")
		Expect(route.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "standard")))
		Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
		Expect(route.Spec.PassThrough).To(BeTrue())
		Expect(route.Spec.Methods).To(Equal([]string{"HEAD"}))
		Expect(route.Spec.RequestTermination).To(Equal(&gatewayapi.RequestTermination{StatusCode: 200}))
		Expect(route.Spec.Hostnames).To(Equal([]string{"test-stargate.de"}))
		// The fixture zone is World-visible; the probe path has no /spacegate prefix.
		Expect(route.Spec.Paths).To(Equal([]string{"/zone-health"}))
		Expect(route.Spec.Backend.Upstreams).To(Equal([]gatewayapi.Upstream{{
			Scheme: "http", Hostname: "localhost", Port: jumperIdentityPort, Path: "/api/v1/zone-health",
		}}))
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, domainName))
		Expect(route.Labels).To(HaveKeyWithValue(config.OwnerUidLabelKey, string(zone.UID)))
		Expect(zone.Status.ManagedRoutes).To(BeEmpty())
	})

	It("is idempotent across reconciliations", func() {
		reconcile()
		before := getZoneHealthRoute("standard")

		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

		after := getZoneHealthRoute("standard")
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
	})

	It("covers the hostnames of all presets of the gateway when DTC is enabled", func() {
		zone.Spec.Presets = append(zone.Spec.Presets, adminv1.Preset{
			Name: "consumer-failover", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary",
			Urls:     []adminv1.UrlConfig{{Hostname: "failover.example.com", BasePath: "/"}},
			Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}},
		})

		reconcile()

		route := getZoneHealthRoute("standard")
		Expect(route.Spec.Hostnames).To(Equal([]string{"failover.example.com", "test-stargate.de"}))
		Expect(route.Spec.Paths).To(Equal([]string{"/zone-health"}))
		Expect(route.Spec.Methods).To(Equal([]string{"HEAD"}))
	})

	It("serves the probe under the preset base path", func() {
		zone.Spec.Presets[0].Urls[0].BasePath = "/env1"

		reconcile()

		Expect(getZoneHealthRoute("standard").Spec.Paths).To(Equal([]string{"/env1/zone-health"}))
	})

	DescribeTable("blocks invalid route cardinality before creating identity routes",
		func(presets []adminv1.Preset, expected string) {
			handler := &ZoneHandler{}
			Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
			markSubResourcesReady(zone)
			zone.Spec.Presets = append(zone.Spec.Presets, presets...)

			err := handler.CreateOrUpdate(newTestContext(zone), zone)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue(), "expected BlockedError, got %v", err)
			Expect(err).To(MatchError(ContainSubstring(expected)))

			issuer := client.ObjectKey{
				Namespace: zone.Status.Namespace,
				Name:      naming.ForGateway(zone, "standard") + "--" + zone.Status.RealmName + "--issuer",
			}
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, issuer, &gatewayapi.Route{}))).To(BeTrue())
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, zoneHealthKey("standard"), &gatewayapi.Route{}))).To(BeTrue())
		},
		Entry("too many hostnames", func() []adminv1.Preset {
			presets := make([]adminv1.Preset, 4)
			for i := range presets {
				urls := make([]adminv1.UrlConfig, 5)
				for j := range urls {
					urls[j] = adminv1.UrlConfig{Hostname: fmt.Sprintf("host-%d-%d.example.com", i, j), BasePath: "/"}
				}
				presets[i] = adminv1.Preset{
					Name: fmt.Sprintf("preset-%d", i), Type: adminv1.GatewayTypeAPI, GatewayRef: "standard",
					IdentityProviderRef: "primary", Urls: urls,
				}
			}
			return presets
		}(), "hostnames"),
		Entry("too many paths", func() []adminv1.Preset {
			presets := make([]adminv1.Preset, 2)
			for i := range presets {
				urls := make([]adminv1.UrlConfig, 5)
				for j := range urls {
					urls[j] = adminv1.UrlConfig{Hostname: "test-stargate.de", BasePath: fmt.Sprintf("/path-%d-%d", i, j)}
				}
				presets[i] = adminv1.Preset{
					Name: fmt.Sprintf("preset-%d", i), Type: adminv1.GatewayTypeAPI, GatewayRef: "standard",
					IdentityProviderRef: "primary", Urls: urls,
				}
			}
			return presets
		}(), "paths"),
	)

	It("counts distinct joined paths, not raw base paths", func() {
		zone.Spec.Presets[0].Urls = nil
		for i := range 5 {
			zone.Spec.Presets[0].Urls = append(zone.Spec.Presets[0].Urls,
				adminv1.UrlConfig{Hostname: "test-stargate.de", BasePath: fmt.Sprintf("/path-%d", i)})
		}
		other := adminv1.Preset{
			Name: "other", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary",
		}
		for i := 5; i < 10; i++ {
			other.Urls = append(other.Urls, adminv1.UrlConfig{
				Hostname: "test-stargate.de", BasePath: fmt.Sprintf("/path-%d", i),
			})
		}
		zone.Spec.Presets = append(zone.Spec.Presets, other, adminv1.Preset{
			Name: "duplicate", Type: adminv1.GatewayTypeAPI, GatewayRef: "standard", IdentityProviderRef: "primary",
			Urls: []adminv1.UrlConfig{{Hostname: "test-stargate.de", BasePath: "/path-9/"}},
		})

		reconcile()

		Expect(getZoneHealthRoute("standard").Spec.Paths).To(HaveLen(maxRoutePaths))
		Expect(getZoneHealthRoute("standard").Spec.Paths).To(ContainElement("/path-9/zone-health"))
		issuer := &gatewayapi.Route{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: zone.Status.Namespace,
			Name:      naming.ForGateway(zone, "standard") + "--" + zone.Status.RealmName + "--issuer",
		}, issuer)).To(Succeed())
		Expect(issuer.Spec.Paths).To(HaveLen(maxRoutePaths))
	})

	It("creates one route per gateway and removes it with the gateway", func() {
		addAiGateway()
		reconcile()

		standard := getZoneHealthRoute("standard")
		ai := getZoneHealthRoute("ai")
		Expect(standard.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "standard")))
		Expect(ai.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "ai")))
		Expect(ai.Spec.Hostnames).To(Equal([]string{"ai.example.com"}))
		Expect(ai.Spec.RequestTermination).To(Equal(&gatewayapi.RequestTermination{StatusCode: 200}))

		zone.Spec.Gateways = zone.Spec.Gateways[:1]
		zone.Spec.Presets = zone.Spec.Presets[:1]
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, zoneHealthKey("ai"), &gatewayapi.Route{}))).To(BeTrue())
		getZoneHealthRoute("standard")
	})

	It("keeps the route when managed routes are removed", func() {
		zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{
			{Name: "proxy", Path: "/proxy", Url: "https://backend.example.com", Type: adminv1.ManagedRouteTypeProxy},
		}}
		reconcile()
		getZoneHealthRoute("standard")

		zone.Spec.ManagedRoutes = nil
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())

		getZoneHealthRoute("standard")
		proxyKey := client.ObjectKey{Namespace: zone.Status.Namespace, Name: naming.ForGateway(zone, "standard") + "--proxy"}
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, proxyKey, &gatewayapi.Route{}))).To(BeTrue())
	})

	Describe("createZoneHealthRoutes", func() {
		var hc *HandlingContext

		BeforeEach(func() {
			hc = newTestHandlingContext(newTestContext(zone), zone)
			hc.Gateways = map[string]*gatewayapi.Gateway{
				"standard": {ObjectMeta: metav1.ObjectMeta{Name: "standard", Namespace: zone.Status.Namespace}},
			}
		})

		expectBlocked := func(err error, msg string) {
			GinkgoHelper()
			var blocked ctrlerrors.BlockedError
			Expect(err).To(MatchError(ContainSubstring(msg)))
			Expect(errors.As(err, &blocked)).To(BeTrue())
		}

		It("returns a blocked error when a gateway is absent from the handling context", func() {
			hc.Gateways = map[string]*gatewayapi.Gateway{}
			expectBlocked(createZoneHealthRoutes(newTestContext(zone), hc), "cannot resolve gateway")
		})

		It("returns a blocked error when the gateway has no preset hostnames", func() {
			zone.Spec.Presets[0].GatewayRef = "other"
			expectBlocked(createZoneHealthRoutes(newTestContext(zone), hc), "has no preset hostnames")
		})

		It("returns a blocked error when the route limits are exceeded", func() {
			urls := make([]adminv1.UrlConfig, 0, maxRoutePaths+1)
			for i := range maxRoutePaths + 1 {
				urls = append(urls, adminv1.UrlConfig{Hostname: "test-stargate.de", BasePath: fmt.Sprintf("/p%d", i)})
			}
			zone.Spec.Presets[0].Urls = urls
			expectBlocked(validateGatewayRouteInputs(newTestContext(zone), hc), "a route requires 1 to")
		})
	})
})
