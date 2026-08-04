// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zone Handler Steps", func() {
	var (
		zone    *adminv1.Zone
		zoneIdx int
	)

	BeforeEach(func() {
		zoneIdx++
		zone = newTestZone(fmt.Sprintf("zone-%d", zoneIdx))
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createIdentityProvider
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createIdentityProvider", func() {
		It("should create an IdentityProvider with correct spec", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			err := createIdentityProvider(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.IdentityProvider).NotTo(BeNil())
			Expect(hc.IdentityProvider.Spec.AdminUrl).To(Equal("https://test-iris.de/auth/admin/realms"))
			Expect(hc.IdentityProvider.Spec.AdminClientId).To(Equal("test-idp-admin-id"))
			Expect(hc.IdentityProvider.Spec.AdminUserName).To(Equal("test-idp-admin-username"))
			Expect(hc.IdentityProvider.Spec.AdminPassword).To(Equal("test-idp-admin-password"))
		})

		It("should populate IdentityProvider status reference", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(zone.Status.IdentityProvider).NotTo(BeNil())
			Expect(zone.Status.IdentityProvider.Name).To(Equal(hc.IdentityProvider.Name))
		})

		It("should derive admin URL from base when not explicitly set", func() {
			zone.Spec.IdentityProvider.Admin.Url = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			err := createIdentityProvider(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.IdentityProvider.Spec.AdminUrl).To(Equal("https://test-iris.de/auth/admin/realms"))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createDefaultIdentityRealm
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createDefaultIdentityRealm", func() {
		It("should create realm with claims and no secret rotation by default", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())

			err := createDefaultIdentityRealm(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.DefaultIdentityRealm).NotTo(BeNil())
			Expect(hc.DefaultIdentityRealm.Spec.Claims).To(HaveLen(3))
			Expect(hc.DefaultIdentityRealm.Spec.SecretRotation).To(BeNil())
			Expect(zone.Status.IdentityRealm).NotTo(BeNil())
		})

		It("should enable secret rotation when configured", func() {
			zone.Spec.IdentityProvider.SecretRotation = &adminv1.SecretRotationConfig{
				Enabled:          true,
				GracePeriod:      metav1.Duration{Duration: 3600},
				ExpirationPeriod: metav1.Duration{Duration: 7200},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())

			err := createDefaultIdentityRealm(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.DefaultIdentityRealm.Spec.SecretRotation).NotTo(BeNil())
			Expect(hc.DefaultIdentityRealm.Spec.SecretRotation.GracePeriod).To(Equal(metav1.Duration{Duration: 3600}))
			Expect(zone.IsFeatureEnabled(adminv1.FeatureSecretRotation)).To(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createInternalIdentityRealm
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createInternalIdentityRealm", func() {
		It("should never enable secret rotation even when configured on zone", func() {
			zone.Spec.IdentityProvider.SecretRotation = &adminv1.SecretRotationConfig{
				Enabled:          true,
				GracePeriod:      metav1.Duration{Duration: 3600},
				ExpirationPeriod: metav1.Duration{Duration: 7200},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())

			err := createInternalIdentityRealm(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.InternalIdentityRealm).NotTo(BeNil())
			Expect(hc.InternalIdentityRealm.Name).To(Equal("rover"))
			Expect(hc.InternalIdentityRealm.Spec.SecretRotation).To(BeNil())
		})

		It("should populate InternalIdentityRealm status reference", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())

			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(zone.Status.InternalIdentityRealm).NotTo(BeNil())
			Expect(zone.Status.InternalIdentityRealm.Name).To(Equal("rover"))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createGatewayAdminClient
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createGatewayAdminClient", func() {
		It("should create admin client with provided secret", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())

			err := createGatewayAdminClient(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayAdminClient).NotTo(BeNil())
			Expect(hc.GatewayAdminClient.Spec.ClientId).To(Equal("rover"))
			Expect(hc.GatewayAdminClient.Spec.ClientSecret).To(Equal("test-gateway-admin-secret"))
			Expect(hc.GatewayAdminClient.Spec.Realm.Name).To(Equal("rover"))
			Expect(zone.Status.GatewayAdminClient).NotTo(BeNil())
		})

		It("should return blocked error when client secret is nil", func() {
			zone.Spec.Gateway.Admin.ClientSecret = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())

			err := createGatewayAdminClient(testCtx, hc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gateway admin client secret must be provided"))
		})

		It("should preserve secret on re-reconcile (idempotency)", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			firstSecret := hc.GatewayAdminClient.Spec.ClientSecret

			// Second reconcile
			testCtx2 := newTestContext(zone)
			hc2 := newTestHandlingContext(testCtx2, zone)
			hc2.IdentityProvider = hc.IdentityProvider
			hc2.InternalIdentityRealm = hc.InternalIdentityRealm

			Expect(createGatewayAdminClient(testCtx2, hc2)).To(Succeed())
			Expect(hc2.GatewayAdminClient.Spec.ClientSecret).To(Equal(firstSecret))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createGateway
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createGateway", func() {
		It("should create gateway with correct admin config", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())

			err := createGateway(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.Gateway).NotTo(BeNil())
			Expect(hc.Gateway.Spec.Admin.Url).To(Equal("https://test-stargate.de/admin-api"))
			Expect(hc.Gateway.Spec.Admin.ClientSecret).To(Equal("test-gateway-admin-secret"))
			Expect(hc.Gateway.Spec.Redis.Host).To(Equal("http://test-redis.de/"))
			Expect(hc.Gateway.Spec.Redis.Port).To(Equal(123))
		})

		It("should use rover client credentials from admin client", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())

			err := createGateway(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.Gateway.Spec.Admin.ClientId).To(Equal("rover"))
			Expect(hc.Gateway.Spec.Admin.ClientSecret).To(Equal(hc.GatewayAdminClient.Spec.ClientSecret))
			Expect(hc.Gateway.Spec.Admin.IssuerUrl).To(Equal("https://test-iris.de/auth/realms/rover"))
		})

		It("should populate Gateway status reference", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())

			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(zone.Status.Gateway).NotTo(BeNil())
			Expect(zone.Status.Gateway.Name).To(Equal("gateway"))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createGatewayConsumer
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createGatewayConsumer", func() {
		It("should reference the correct gateway", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())

			err := createGatewayConsumer(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayConsumer).NotTo(BeNil())
			Expect(hc.GatewayConsumer.Spec.Gateway.Name).To(Equal(hc.Gateway.Name))
			Expect(hc.GatewayConsumer.Spec.Name).To(Equal("gateway"))
			Expect(zone.Status.GatewayConsumer).NotTo(BeNil())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: reconcileInternalRoutes
	// ─────────────────────────────────────────────────────────────────────────

	Describe("reconcileInternalRoutes", func() {
		It("should create TeamAPI routes with authentication disabled", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "team-api",
					Path: "/team/api/v1",
					Url:  "https://team-backend.de/api",
					Type: adminv1.ManagedRouteTypeTeamAPI,
				}},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())

			err := reconcileInternalRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))
			Expect(zone.Status.TeamApiIdentityRealm).NotTo(BeNil())

			// Verify the route in k8s
			route := &gatewayapi.Route{}
			routeKey := client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--team-api",
			}
			Expect(k8sClient.Get(ctx, routeKey, route)).To(Succeed())
			Expect(route.Spec.PassThrough).To(BeFalse())
			Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
			Expect(route.Spec.GatewayRef.Name).To(Equal("gateway"))
		})

		It("should create Proxy routes with full passthrough", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "test-proxy",
					Path: "/proxy/path",
					Url:  "https://proxy-upstream.de/backend",
					Type: adminv1.ManagedRouteTypeProxy,
				}},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())

			err := reconcileInternalRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))

			// Verify route is passthrough without security
			route := &gatewayapi.Route{}
			routeKey := client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--test-proxy",
			}
			Expect(k8sClient.Get(ctx, routeKey, route)).To(Succeed())
			Expect(route.Spec.PassThrough).To(BeTrue())
			Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
			Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("proxy-upstream.de"))
			Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/backend"))
		})

		It("should be a no-op when ManagedRoutes is nil", func() {
			zone.Spec.ManagedRoutes = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())

			err := reconcileInternalRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.ManagedRoutes).To(BeNil())
			Expect(zone.Status.TeamApiIdentityRealm).To(BeNil())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: createIdentityRoutes
	// ─────────────────────────────────────────────────────────────────────────

	Describe("createIdentityRoutes", func() {
		It("should create issuer, certs, and discovery routes for default realm", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())

			err := createIdentityRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())

			// Verify issuer route exists
			issuerRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + hc.DefaultIdentityRealm.Name + "--issuer",
			}, issuerRoute)).To(Succeed())
			Expect(issuerRoute.Spec.PassThrough).To(BeTrue())
			Expect(issuerRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8081)))

			// Verify certs route exists
			certsRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + hc.DefaultIdentityRealm.Name + "--certs",
			}, certsRoute)).To(Succeed())
			Expect(certsRoute.Spec.PassThrough).To(BeTrue())

			// Verify discovery route exists
			discoveryRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + hc.DefaultIdentityRealm.Name + "--discovery",
			}, discoveryRoute)).To(Succeed())
			Expect(discoveryRoute.Spec.PassThrough).To(BeTrue())
		})

		It("should add spacegate prefix for World visibility", func() {
			zone.Spec.Visibility = adminv1.ZoneVisibilityWorld
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())

			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())

			issuerRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + hc.DefaultIdentityRealm.Name + "--issuer",
			}, issuerRoute)).To(Succeed())

			// Paths should contain /spacegate prefix
			Expect(issuerRoute.Spec.Paths).NotTo(BeEmpty())
			Expect(issuerRoute.Spec.Paths[0]).To(ContainSubstring("/spacegate/"))
		})

		It("should NOT add spacegate prefix for Enterprise visibility", func() {
			zone.Spec.Visibility = adminv1.ZoneVisibilityEnterprise
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())

			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())

			issuerRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + hc.DefaultIdentityRealm.Name + "--issuer",
			}, issuerRoute)).To(Succeed())

			// Paths should NOT contain /spacegate prefix
			Expect(issuerRoute.Spec.Paths).NotTo(BeEmpty())
			Expect(issuerRoute.Spec.Paths[0]).NotTo(ContainSubstring("/spacegate/"))
		})

		It("should also create identity routes for team-api realm when team routes exist", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "team-api",
					Path: "/team/v1",
					Url:  "https://team.de/v1",
					Type: adminv1.ManagedRouteTypeTeamAPI,
				}},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())

			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())

			// Should have routes for both default realm and team-api realm
			teamRealmName := hc.TeamApiIdentityRealm.Name
			teamIssuerRoute := &gatewayapi.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--" + teamRealmName + "--issuer",
			}, teamIssuerRoute)).To(Succeed())
			Expect(teamIssuerRoute.Spec.PassThrough).To(BeTrue())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: cleanupStaleRoutes
	// ─────────────────────────────────────────────────────────────────────────

	Describe("cleanupStaleRoutes", func() {
		It("should remove routes that are no longer managed", func() {
			// First reconcile with two routes
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{
					{Name: "route-a", Path: "/a", Url: "https://a.de/a", Type: adminv1.ManagedRouteTypeProxy},
					{Name: "route-b", Path: "/b", Url: "https://b.de/b", Type: adminv1.ManagedRouteTypeProxy},
				},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())
			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())
			Expect(cleanupStaleRoutes(testCtx, hc)).To(Succeed())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(2))

			// Second reconcile: remove route-b from spec
			zone.Spec.ManagedRoutes.Routes = zone.Spec.ManagedRoutes.Routes[:1]
			zone.Status.ManagedRoutes = nil

			testCtx2 := newTestContext(zone)
			hc2 := newTestHandlingContext(testCtx2, zone)
			hc2.IdentityProvider = hc.IdentityProvider
			hc2.DefaultIdentityRealm = hc.DefaultIdentityRealm
			hc2.InternalIdentityRealm = hc.InternalIdentityRealm
			hc2.GatewayAdminClient = hc.GatewayAdminClient
			hc2.Gateway = hc.Gateway
			hc2.GatewayConsumer = hc.GatewayConsumer
			Expect(reconcileInternalRoutes(testCtx2, hc2)).To(Succeed())
			Expect(createIdentityRoutes(testCtx2, hc2)).To(Succeed())
			Expect(cleanupStaleRoutes(testCtx2, hc2)).To(Succeed())

			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))

			// Verify route-b is gone
			staleRoute := &gatewayapi.Route{}
			staleRouteKey := client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      "gateway--route-b",
			}
			err := k8sClient.Get(ctx, staleRouteKey, staleRoute)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: populateLinks
	// ─────────────────────────────────────────────────────────────────────────

	Describe("populateLinks", func() {
		It("should compute all URLs correctly for World visibility", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())
			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())
			Expect(cleanupStaleRoutes(testCtx, hc)).To(Succeed())

			err := populateLinks(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.Links.Url).To(Equal("https://test-stargate.de/"))
			Expect(zone.Status.Links.Issuer).To(Equal("https://test-iris.de/auth/realms/test"))
			// World visibility: LMS issuer includes /spacegate prefix
			Expect(zone.Status.Links.LmsIssuer).To(Equal("https://test-stargate.de/spacegate/auth/realms/test"))
			Expect(zone.Status.Links.InternalIssuer).To(Equal("https://test-iris.de/auth/realms/" + naming.ForInternalIdentityRealm()))
		})

		It("should compute LMS issuer without spacegate prefix for Enterprise visibility", func() {
			zone.Spec.Visibility = adminv1.ZoneVisibilityEnterprise
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())
			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())
			Expect(cleanupStaleRoutes(testCtx, hc)).To(Succeed())

			err := populateLinks(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.Links.Url).To(Equal("https://test-stargate.de/"))
			Expect(zone.Status.Links.LmsIssuer).To(Equal("https://test-stargate.de/auth/realms/test"))
		})

		It("should clear PermissionsUrl when feature disabled", func() {
			zone.Spec.Permissions = &adminv1.PermissionsConfig{
				ApiBasePath: "/eni/chevron/v2/permission",
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGatewayAdminClient(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createGatewayConsumer(testCtx, hc)).To(Succeed())
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())
			Expect(createIdentityRoutes(testCtx, hc)).To(Succeed())
			Expect(cleanupStaleRoutes(testCtx, hc)).To(Succeed())

			err := populateLinks(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.Links.PermissionsUrl).To(BeEmpty())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Step: populateRealmName
	// ─────────────────────────────────────────────────────────────────────────

	Describe("populateRealmName", func() {
		It("should default realmName to environment name when Spec.RealmName is empty", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			Expect(hc.Environment.Spec.RealmName).To(BeEmpty())

			err := populateRealmName(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.RealmName).To(Equal(testEnvironment))
		})

		It("should use Spec.RealmName when it is set", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			hc.Environment.Spec.RealmName = "custom-realm"

			err := populateRealmName(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.RealmName).To(Equal("custom-realm"))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Full Pipeline (CreateOrUpdate)
	// ─────────────────────────────────────────────────────────────────────────

	Describe("Full pipeline (CreateOrUpdate)", func() {
		It("should run all steps and produce NotReady on first pass, Ready on second", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "team-api",
					Path: "/team/v1",
					Url:  "https://team.de/v1",
					Type: adminv1.ManagedRouteTypeTeamAPI,
				}},
			}
			handler := &ZoneHandler{}

			// First pass: all resources are new -> AnyChanged() is true -> NotReady
			testCtx := newTestContext(zone)
			err := handler.CreateOrUpdate(testCtx, zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			// Status refs are all populated even on first pass
			Expect(zone.Status.Namespace).To(Equal(strings.ToLower(fmt.Sprintf("%s--%s", testEnvironment, zone.Name))))
			Expect(zone.Status.IdentityProvider).NotTo(BeNil())
			Expect(zone.Status.IdentityRealm).NotTo(BeNil())
			Expect(zone.Status.InternalIdentityRealm).NotTo(BeNil())
			Expect(zone.Status.Gateway).NotTo(BeNil())
			Expect(zone.Status.GatewayConsumer).NotTo(BeNil())
			Expect(zone.Status.TeamApiIdentityRealm).NotTo(BeNil())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))

			// Links
			Expect(zone.Status.Links.Url).NotTo(BeEmpty())
			Expect(zone.Status.Links.Issuer).NotTo(BeEmpty())
			Expect(zone.Status.Links.LmsIssuer).NotTo(BeEmpty())
			Expect(zone.Status.Links.TeamIssuer).NotTo(BeEmpty())

			// Second pass: nothing changed -> Ready
			testCtx2 := newTestContext(zone)
			err = handler.CreateOrUpdate(testCtx2, zone)
			Expect(err).NotTo(HaveOccurred())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		})

		It("should be idempotent across multiple invocations", func() {
			handler := &ZoneHandler{}

			// First run: resources created -> NotReady
			testCtx := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx, zone)).To(Succeed())
			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			firstNamespace := zone.Status.Namespace

			// Second run: nothing changed -> Ready
			testCtx2 := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx2, zone)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			Expect(zone.Status.Namespace).To(Equal(firstNamespace))

			// Third run: still nothing changed -> still Ready
			testCtx3 := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx3, zone)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		})

		It("should handle Enterprise zone without spacegate prefix", func() {
			zone.Spec.Visibility = adminv1.ZoneVisibilityEnterprise
			zone.Spec.ManagedRoutes = nil
			handler := &ZoneHandler{}

			// First pass
			testCtx := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx, zone)).To(Succeed())

			// Second pass -> Ready
			testCtx2 := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx2, zone)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			// LMS issuer should not have /spacegate prefix
			Expect(zone.Status.Links.LmsIssuer).NotTo(ContainSubstring("/spacegate"))
		})
	})
})
