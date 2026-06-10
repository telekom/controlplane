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
	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

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
		// Create zone in k8s so it gets a UID (needed for owner labels)
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		// Re-fetch to get UID
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
	})

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

			// Status should be populated
			Expect(zone.Status.IdentityProvider).NotTo(BeNil())
			Expect(zone.Status.IdentityProvider.Name).To(Equal(hc.IdentityProvider.Name))
		})

		It("should derive admin URL from base when not explicitly set", func() {
			zone.Spec.IdentityProvider.Admin.Url = ""
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			err := createIdentityProvider(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.IdentityProvider.Spec.AdminUrl).To(Equal("https://test-iris.de/auth/admin/realms"))
		})
	})

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

			// Status should be populated
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

	Describe("createInternalIdentityRealm", func() {
		It("should never enable secret rotation", func() {
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
	})

	Describe("createGatewayAdminClient", func() {
		It("should create rover client when not externally managed", func() {
			// Remove all external admin config to make it managed
			zone.Spec.Gateway.Admin.ClientId = nil
			zone.Spec.Gateway.Admin.ClientSecret = nil
			zone.Spec.Gateway.Admin.TokenUrl = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())

			err := createGatewayAdminClient(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayAdminClient).NotTo(BeNil())
			Expect(hc.GatewayAdminClient.Spec.ClientId).To(Equal("rover"))
			Expect(hc.GatewayAdminClient.Spec.ClientSecret).NotTo(BeEmpty())
			Expect(hc.GatewayAdminClient.Spec.Realm.Name).To(Equal("rover"))
			Expect(zone.Status.GatewayAdminClient).NotTo(BeNil())
		})

		It("should skip when externally managed", func() {
			// Default newTestZone has ClientSecret set -> externally managed
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())

			err := createGatewayAdminClient(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayAdminClient).To(BeNil())
			Expect(zone.Status.GatewayAdminClient).To(BeNil())
		})

		It("should preserve secret on re-reconcile (idempotency)", func() {
			zone.Spec.Gateway.Admin.ClientId = nil
			zone.Spec.Gateway.Admin.ClientSecret = nil
			zone.Spec.Gateway.Admin.TokenUrl = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createInternalIdentityRealm(testCtx, hc)).To(Succeed())

			// First reconcile
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

	Describe("createGatewayClient", func() {
		It("should create client with generated secret", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())

			err := createGatewayClient(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayClient).NotTo(BeNil())
			Expect(hc.GatewayClient.Spec.ClientId).To(Equal("gateway"))
			Expect(hc.GatewayClient.Spec.ClientSecret).NotTo(BeEmpty())
		})

		It("should preserve secret on re-reconcile (idempotency)", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())

			// First reconcile
			Expect(createGatewayClient(testCtx, hc)).To(Succeed())
			firstSecret := hc.GatewayClient.Spec.ClientSecret

			// Second reconcile — simulate fresh hc but same zone
			testCtx2 := newTestContext(zone)
			hc2 := newTestHandlingContext(testCtx2, zone)
			hc2.IdentityProvider = hc.IdentityProvider
			hc2.DefaultIdentityRealm = hc.DefaultIdentityRealm

			Expect(createGatewayClient(testCtx2, hc2)).To(Succeed())
			Expect(hc2.GatewayClient.Spec.ClientSecret).To(Equal(firstSecret))
		})
	})

	Describe("createGateway", func() {
		It("should create with externally managed admin config", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			err := createGateway(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.Gateway).NotTo(BeNil())
			Expect(hc.Gateway.Spec.Admin.Url).To(Equal("https://test-stargate.de/admin-api"))
			Expect(hc.Gateway.Spec.Admin.ClientSecret).To(Equal("test-gateway-admin-secret"))
			Expect(hc.Gateway.Spec.Redis.Host).To(Equal("http://test-redis.de/"))
			Expect(hc.Gateway.Spec.Redis.Port).To(Equal(123))
		})

		It("should use managed rover client credentials when not externally managed", func() {
			zone.Spec.Gateway.Admin.ClientId = nil
			zone.Spec.Gateway.Admin.ClientSecret = nil
			zone.Spec.Gateway.Admin.TokenUrl = nil
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

		It("should derive admin URL when not set", func() {
			zone.Spec.Gateway.Admin.Url = ""
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)

			err := createGateway(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.Gateway.Spec.Admin.Url).To(Equal("https://test-stargate.de/admin-api"))
		})
	})

	Describe("createDefaultGatewayRealm", func() {
		It("should add SpaceGate overwrites for World visibility", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createGateway(testCtx, hc)).To(Succeed())

			err := createDefaultGatewayRealm(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.DefaultGatewayRealm).NotTo(BeNil())
			Expect(hc.DefaultGatewayRealm.Spec.RouteOverwrites).To(HaveLen(3))
			Expect(hc.DefaultGatewayRealm.Spec.RouteOverwrites[0].PathPrefix).To(Equal("/spacegate"))
		})

		It("should NOT add route overwrites for Enterprise visibility", func() {
			zone.Spec.Visibility = adminv1.ZoneVisibilityEnterprise
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createGateway(testCtx, hc)).To(Succeed())

			err := createDefaultGatewayRealm(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.DefaultGatewayRealm.Spec.RouteOverwrites).To(BeEmpty())
		})
	})

	Describe("createGatewayConsumer", func() {
		It("should reference the correct realm", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			err := createGatewayConsumer(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(hc.GatewayConsumer).NotTo(BeNil())
			Expect(hc.GatewayConsumer.Spec.Realm.Name).To(Equal(hc.DefaultGatewayRealm.Name))
			Expect(hc.GatewayConsumer.Spec.Name).To(Equal("gateway"))
		})
	})

	Describe("reconcileInternalRoutes", func() {
		It("should create TeamAPI routes", func() {
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
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			err := reconcileInternalRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))
			Expect(zone.Status.TeamApiGatewayRealm).NotTo(BeNil())

			// Verify the route exists in k8s
			route := &gatewayapi.Route{}
			routeKey := client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      fmt.Sprintf("team-%s--team-api", testEnvironment),
			}
			Expect(k8sClient.Get(ctx, routeKey, route)).To(Succeed())
			Expect(route.Spec.PassThrough).To(BeFalse())
			Expect(route.Spec.Security).NotTo(BeNil())
			Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
		})

		It("should be a no-op when ManagedRoutes is nil", func() {
			zone.Spec.ManagedRoutes = nil
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			err := reconcileInternalRoutes(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.ManagedRoutes).To(BeNil())
			Expect(zone.Status.TeamApiGatewayRealm).To(BeNil())
		})

		It("should clean up stale routes removed from spec", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{
					{Name: "route-a", Path: "/a", Url: "https://a.de/a", Type: adminv1.ManagedRouteTypeProxy},
					{Name: "route-b", Path: "/b", Url: "https://b.de/b", Type: adminv1.ManagedRouteTypeProxy},
				},
			}
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			// First reconcile — both routes created
			Expect(reconcileInternalRoutes(testCtx, hc)).To(Succeed())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(2))

			// Remove route-b from spec
			zone.Spec.ManagedRoutes.Routes = zone.Spec.ManagedRoutes.Routes[:1]
			zone.Status.ManagedRoutes = nil

			// Second reconcile with fresh janitor
			testCtx2 := newTestContext(zone)
			hc2 := newTestHandlingContext(testCtx2, zone)
			hc2.IdentityProvider = hc.IdentityProvider
			hc2.Gateway = hc.Gateway
			hc2.DefaultGatewayRealm = hc.DefaultGatewayRealm

			Expect(reconcileInternalRoutes(testCtx2, hc2)).To(Succeed())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))

			// Verify route-b is gone
			staleRoute := &gatewayapi.Route{}
			staleRouteKey := client.ObjectKey{
				Namespace: hc.Namespace.Name,
				Name:      hc.DefaultGatewayRealm.Name + "--route-b",
			}
			err := k8sClient.Get(ctx, staleRouteKey, staleRoute)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	Describe("populateLinks", func() {
		It("should compute all URLs correctly", func() {
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			err := populateLinks(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.Links.Url).To(Equal("https://test-stargate.de/"))
			Expect(zone.Status.Links.Issuer).To(Equal("https://test-iris.de/auth/realms/test"))
			Expect(zone.Status.Links.LmsIssuer).To(Equal("https://test-stargate.de/auth/realms/test"))
		})

		It("should clear PermissionsUrl when feature disabled", func() {
			zone.Spec.Permissions = &adminv1.PermissionsConfig{
				ApiBasePath: "/eni/chevron/v2/permission",
			}
			// FeaturePermission is not enabled globally in tests
			testCtx := newTestContext(zone)
			hc := newTestHandlingContext(testCtx, zone)
			Expect(createIdentityProvider(testCtx, hc)).To(Succeed())
			Expect(createDefaultIdentityRealm(testCtx, hc)).To(Succeed())
			Expect(createGateway(testCtx, hc)).To(Succeed())
			Expect(createDefaultGatewayRealm(testCtx, hc)).To(Succeed())

			err := populateLinks(testCtx, hc)
			Expect(err).NotTo(HaveOccurred())
			Expect(zone.Status.Links.PermissionsUrl).To(BeEmpty())
		})
	})

	Describe("Full pipeline (CreateOrUpdate)", func() {
		It("should run all steps and produce a Ready zone", func() {
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "team-api",
					Path: "/team/v1",
					Url:  "https://team.de/v1",
					Type: adminv1.ManagedRouteTypeTeamAPI,
				}},
			}
			handler := &ZoneHandler{}

			// First pass: all resources are new → AnyChanged() is true → NotReady
			testCtx := newTestContext(zone)
			err := handler.CreateOrUpdate(testCtx, zone)
			Expect(err).NotTo(HaveOccurred())

			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			// Status refs are all populated even on first pass
			Expect(zone.Status.Namespace).To(Equal(strings.ToLower(fmt.Sprintf("%s--%s", testEnvironment, zone.Name))))
			Expect(zone.Status.IdentityProvider).NotTo(BeNil())
			Expect(zone.Status.IdentityRealm).NotTo(BeNil())
			Expect(zone.Status.InternalIdentityRealm).NotTo(BeNil())
			Expect(zone.Status.GatewayClient).NotTo(BeNil())
			Expect(zone.Status.Gateway).NotTo(BeNil())
			Expect(zone.Status.GatewayRealm).NotTo(BeNil())
			Expect(zone.Status.GatewayConsumer).NotTo(BeNil())
			Expect(zone.Status.TeamApiGatewayRealm).NotTo(BeNil())
			Expect(zone.Status.ManagedRoutes).To(HaveLen(1))

			// Links
			Expect(zone.Status.Links.Url).NotTo(BeEmpty())
			Expect(zone.Status.Links.Issuer).NotTo(BeEmpty())
			Expect(zone.Status.Links.LmsIssuer).NotTo(BeEmpty())
			Expect(zone.Status.Links.TeamIssuer).NotTo(BeEmpty())

			// Second pass: nothing changed → Ready
			testCtx2 := newTestContext(zone)
			err = handler.CreateOrUpdate(testCtx2, zone)
			Expect(err).NotTo(HaveOccurred())

			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeProcessing)).To(BeTrue())
		})

		It("should be idempotent on second invocation", func() {
			handler := &ZoneHandler{}

			// First run: resources created → NotReady
			testCtx := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx, zone)).To(Succeed())
			Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
			firstNamespace := zone.Status.Namespace

			// Second run: nothing changed → Ready
			testCtx2 := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx2, zone)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			Expect(zone.Status.Namespace).To(Equal(firstNamespace))

			// Third run: still nothing changed → still Ready
			testCtx3 := newTestContext(zone)
			Expect(handler.CreateOrUpdate(testCtx3, zone)).To(Succeed())
			Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

			// Verify the gateway client secret didn't change
			gatewayClient := &identityapi.Client{}
			Expect(k8sClient.Get(ctx, zone.Status.GatewayClient.K8s(), gatewayClient)).To(Succeed())
			Expect(gatewayClient.Spec.ClientSecret).NotTo(BeEmpty())
		})
	})
})
