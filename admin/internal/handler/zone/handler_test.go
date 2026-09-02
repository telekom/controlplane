// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zone Handler", func() {
	var zone *adminv1.Zone
	var zoneIdx int

	BeforeEach(func() {
		zoneIdx++
		zone = newTestZone(fmt.Sprintf("zone-%d", zoneIdx))
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), zone)).To(Succeed())
	})

	AfterEach(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed()) })

	It("reconciles named infrastructure and publishes sorted preset status", func() {
		secret := "alternate-secret"
		zone.Spec.Gateways = append(zone.Spec.Gateways, adminv1.GatewayConfig{
			Types: []adminv1.GatewayType{adminv1.GatewayTypeAI},
			Name:  "ai", Admin: adminv1.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "https://ai.example.com/admin-api", ClientSecret: &secret},
		})
		zone.Spec.Presets = append(zone.Spec.Presets,
			adminv1.Preset{Name: "consumer-failover", GatewayRef: "standard", IdentityProviderRef: "primary", TokenUrl: "https://tokens.example.com/failover", Urls: []adminv1.UrlConfig{{Hostname: "failover.example.com", BasePath: "/"}}, Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}}},
			adminv1.Preset{Name: "ai", GatewayRef: "ai", IdentityProviderRef: "primary", Urls: []adminv1.UrlConfig{{Hostname: "ai.example.com", BasePath: "/"}}},
		)

		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(zone.Status.Gateways).To(HaveLen(2))
		Expect(zone.Status.Presets).To(HaveLen(3))
		Expect(zone.Status.Presets[0].Name).To(Equal("ai"))
		defaultStatus, err := zone.Status.GetPreset("default")
		Expect(err).NotTo(HaveOccurred())
		Expect(defaultStatus.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "standard")))
		Expect(defaultStatus.IdentityProviderRef.Name).To(Equal(naming.ForIdentityProvider(zone, "primary")))
		failover, err := zone.Status.GetPreset("consumer-failover")
		Expect(err).NotTo(HaveOccurred())
		Expect(failover.Links.Url).To(Equal("https://failover.example.com/"))
		Expect(failover.Links.TokenUrl).To(Equal("https://tokens.example.com/failover"))
		ai, err := zone.Status.GetPreset("ai")
		Expect(err).NotTo(HaveOccurred())
		Expect(ai.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "ai")))
		Expect(zone.Spec.FeaturesSupported(adminv1.GatewayTypeAI)).To(BeTrue())

		idps := &identityapi.IdentityProviderList{}
		gateways := &gatewayapi.GatewayList{}
		consumers := &gatewayapi.ConsumerList{}
		clients := &identityapi.ClientList{}
		Expect(k8sClient.List(ctx, idps, client.InNamespace(zone.Status.Namespace))).To(Succeed())
		Expect(k8sClient.List(ctx, gateways, client.InNamespace(zone.Status.Namespace))).To(Succeed())
		Expect(k8sClient.List(ctx, consumers, client.InNamespace(zone.Status.Namespace))).To(Succeed())
		Expect(k8sClient.List(ctx, clients, client.InNamespace(zone.Status.Namespace))).To(Succeed())
		Expect(idps.Items).To(HaveLen(1))
		Expect(gateways.Items).To(HaveLen(2))
		Expect(consumers.Items).To(HaveLen(2))
		Expect(consumers.Items).To(ContainElements(
			HaveField("Name", naming.ForGatewayConsumer(zone, "standard")),
			HaveField("Name", naming.ForGatewayConsumer(zone, "ai")),
		))
		Expect(clients.Items).To(HaveLen(1))
		Expect(clients.Items[0].Name).To(Equal(naming.ForGatewayAdminClient(naming.ForIdentityProvider(zone, "primary"))))
		Expect(clients.Items[0].Spec.ClientSecret).To(Equal(secret))

		zone.Spec.Presets = append(zone.Spec.Presets[:1], zone.Spec.Presets[2:]...)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(zone.Status.Presets).To(HaveLen(2))
		Expect(zone.Status.Gateways).To(HaveLen(2))
	})

	It("preserves managed route behavior on the default preset", func() {
		zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{Routes: []adminv1.ManagedRouteConfig{{Name: "proxy", Path: "/proxy", Url: "https://backend.example.com/base", Type: adminv1.ManagedRouteTypeProxy}}}
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		route := &gatewayapi.Route{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: zone.Status.Namespace, Name: naming.ForGateway(zone, "standard") + "--proxy"}, route)).To(Succeed())
		Expect(route.Spec.GatewayRef.Name).To(Equal(naming.ForGateway(zone, "standard")))
		Expect(route.Spec.Hostnames).To(Equal([]string{"test-stargate.de"}))
	})

	It("is ready after identity resources are ready and reconciliation is unchanged", func() {
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(meta.IsStatusConditionFalse(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(zone.Status.Namespace).To(Equal(strings.ToLower(testEnvironment + "--" + zone.Name)))
	})

	It("waits for identity resources before creating gateways", func() {
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(zone.Status.IdentityProvider).NotTo(BeNil())
		Expect(zone.Status.IdentityRealm).NotTo(BeNil())
		Expect(zone.Status.InternalIdentityRealm).NotTo(BeNil())
		Expect(zone.Status.Gateways).To(BeEmpty())

		ready := meta.FindStatusCondition(zone.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready.Reason).To(Equal(condition.ReasonSubResourceNotReady))
		Expect(ready.Message).To(ContainSubstring(zone.Status.IdentityProvider.Name))

		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(zone.Status.Gateways).To(HaveLen(1))
	})

	It("keeps a provisioned zone ready and reports a degraded gateway", func() {
		handler := &ZoneHandler{}
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		markSubResourcesReady(zone)
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(handler.CreateOrUpdate(newTestContext(zone), zone)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

		gateway := &gatewayapi.Gateway{}
		Expect(k8sClient.Get(ctx, zone.Status.Gateways[0].Gateway.K8s(), gateway)).To(Succeed())
		gateway.SetCondition(condition.NewNotReadyCondition("GatewayUnavailable", "health check failed"))
		Expect(k8sClient.Status().Update(ctx, gateway)).To(Succeed())

		recorder := &mock.EventRecorder{}
		testCtx := contextutil.WithRecorder(newTestContext(zone), recorder)
		Expect(handler.CreateOrUpdate(testCtx, zone)).To(Succeed())
		Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
		Expect(recorder.GetEvent(zone, corev1.EventTypeWarning)).To(ContainElement(SatisfyAll(
			HaveField("Reason", condition.ReasonSubResourceNotReady),
			HaveField("Message", And(ContainSubstring("Gateway"), ContainSubstring(gateway.Name), ContainSubstring("health check failed"))),
		)))
	})

	It("returns a blocked error when a gateway admin secret is missing", func() {
		zone.Spec.Gateways[0].Admin.ClientSecret = nil
		hc := newTestHandlingContext(newTestContext(zone), zone)
		hc.IdentityProvider = &identityapi.IdentityProvider{}
		hc.InternalIdentityRealm = &identityapi.Realm{}

		_, err := createGatewayAdminClient(newTestContext(zone), hc, &zone.Spec.Gateways[0])
		var blocked ctrlerrors.BlockedError
		Expect(err).To(HaveOccurred())
		Expect(errors.As(err, &blocked)).To(BeTrue())
	})

	It("returns blocked errors when a preset gateway is absent from the handling context", func() {
		hc := newTestHandlingContext(newTestContext(zone), zone)
		hc.Gateways = map[string]*gatewayapi.Gateway{}
		_, err := createManagedRoute(newTestContext(zone), hc, adminv1.ManagedRouteConfig{Name: "proxy", Url: "https://backend.example.com", Path: "/proxy"}, &zone.Spec.Presets[0], true)
		var blocked ctrlerrors.BlockedError
		Expect(err).To(HaveOccurred())
		Expect(errors.As(err, &blocked)).To(BeTrue())

		err = createIdentityRoute(newTestContext(zone), hc, "realm", identityRouteConfigs[0], &zone.Spec.Presets[0], "")
		Expect(err).To(HaveOccurred())
		Expect(errors.As(err, &blocked)).To(BeTrue())
	})

	DescribeTable("resolves the issuer hostname",
		func(idp adminv1.IdentityProviderConfig, expected string, succeeds bool) {
			hostname, err := issuerHostname(&idp)
			if succeeds {
				Expect(err).NotTo(HaveOccurred())
				Expect(hostname).To(Equal(expected))
				return
			}
			Expect(err).To(HaveOccurred())
		},
		Entry("explicit hostname", adminv1.IdentityProviderConfig{Name: "primary", IssuerHostname: "issuer.example.com"}, "issuer.example.com", true),
		Entry("admin URL fallback", adminv1.IdentityProviderConfig{Name: "primary", Admin: adminv1.IdentityProviderAdminConfig{Url: ptr.To("https://admin.example.com/auth")}}, "admin.example.com", true),
		Entry("missing values", adminv1.IdentityProviderConfig{Name: "primary"}, "", false),
	)
})
