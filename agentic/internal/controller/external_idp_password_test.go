// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consumer username/password with scopes", func() {
	consumerScopes := []string{"consumer.read", "consumer.write"}

	passwordExposure := func(f *scopeFixture, grantType agenticv1.GrantType) *agenticv1.AgenticExposure {
		idp := externalIDPFixture()
		idp.GrantType = grantType
		idp.Basic = &agenticv1.BasicAuthCredentials{Username: "provider-user", Password: "provider-pass"}
		exp := &agenticv1.AgenticExposure{ObjectMeta: f.metadata("exposure"), Spec: agenticv1.AgenticExposureSpec{
			BasePath: f.basePath, Variant: agenticv1.AgenticVariantMCP,
			Upstreams:  []agenticv1.Upstream{{Url: "https://upstream.example.com/mcp", Weight: 100}},
			Visibility: agenticv1.VisibilityEnterprise,
			Approval:   agenticv1.Approval{Strategy: agenticv1.ApprovalStrategySimple},
			Zone:       *ctypes.ObjectRefFromObject(f.zone), Provider: *ctypes.ObjectRefFromObject(f.provider),
			Security: &agenticv1.Security{M2M: &agenticv1.Machine2MachineAuthentication{ExternalIDP: idp}},
		}}
		Expect(k8sClient.Create(ctx, exp)).To(Succeed())
		return exp
	}

	passwordSubscription := func(f *scopeFixture) *agenticv1.AgenticSubscription {
		sub := &agenticv1.AgenticSubscription{ObjectMeta: f.metadata("subscription"), Spec: agenticv1.AgenticSubscriptionSpec{
			BasePath: f.basePath, Zone: *ctypes.ObjectRefFromObject(f.zone),
			Requestor: agenticv1.Requestor{Application: *ctypes.ObjectRefFromObject(f.consumer)},
			Security: &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
				Basic:  &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
				Scopes: consumerScopes,
			}},
		}}
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		return sub
	}

	It("is provisioned when the exposure uses an external IDP password grant", func() {
		f := newScopeFixture()
		f.server("McpServer", nil)
		route := waitExposure(passwordExposure(f, "password"))
		Expect(route.Spec.Security.M2M.Basic).To(BeNil())
		Expect(route.Spec.Security.M2M.ExternalIDP).To(Equal(&gatewayv1.ExternalIdentityProvider{
			TokenEndpoint: tokenEndpoint,
			TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
			GrantType:     gatewayv1.GrantTypePassword,
			Basic:         &gatewayv1.BasicAuthCredentials{Username: "provider-user", Password: "provider-pass"},
		}))
		sub := passwordSubscription(f)
		grantApproval(sub, waitPending(sub))

		consume := waitConsumeRoute(sub, consumerScopes)
		Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
		Expect(consume.Spec.Security.M2M.Client).To(BeNil())
	})

	It("repairs rejected credentials while its exposure waits for the route", func() {
		fixture := newScopeFixture()
		fixture.server("McpServer", nil)
		exposure := passwordExposure(fixture, agenticv1.GrantTypePassword)
		route := waitExposure(exposure)
		sub := fixture.subscription(consumerScopes)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			sub.Spec.Security.M2M.Client = &agenticv1.OAuth2ClientCredentials{ClientId: "invalid-client", ClientSecret: "invalid-secret"}
			g.Expect(k8sClient.Update(ctx, sub)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		grantApproval(sub, waitPending(sub))
		consume := &gatewayv1.ConsumeRoute{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			g.Expect(sub.Status.ConsumeRoute).NotTo(BeNil())
			g.Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
			g.Expect(consume.Spec.Security.M2M.Client).NotTo(BeNil())
		}, timeout, interval).Should(Succeed())
		originalUID := consume.UID
		originalGeneration := consume.Generation

		By("letting route rejection make the active exposure not ready")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			notReady := condition.NewNotReadyCondition(condition.ReasonValidationFailed, "Password route has consumers without usable credentials")
			notReady.ObservedGeneration = route.Generation
			route.SetCondition(notReady)
			route.Status.Consumers = nil
			g.Expect(k8sClient.Status().Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			expectReadyReason(g, exposure, "ChildResourcesNotReady", metav1.ConditionFalse)
			g.Expect(exposure.Status.Active).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		By("repairing the subscription before gateway readiness can recover")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			sub.Spec.Security.M2M.Client = nil
			sub.Spec.Security.M2M.Basic = &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}
			g.Expect(k8sClient.Update(ctx, sub)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consume), consume)).To(Succeed())
			g.Expect(consume.UID).To(Equal(originalUID))
			g.Expect(consume.Generation).To(BeNumerically(">", originalGeneration))
			g.Expect(consume.Spec.Security.M2M.Client).To(BeNil())
			g.Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
			g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(consumerScopes))
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonSubResourceNotReady, metav1.ConditionFalse)
		}, timeout, interval).Should(Succeed())
		updateFixtureStatus(route)
		updateFixtureStatus(consume)
		Eventually(func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonProvisioned, metav1.ConditionTrue)
		}, timeout, interval).Should(Succeed())
	})

	It("does not create a password subscription while its exposure is not ready", func() {
		fixture := newScopeFixture()
		fixture.server("McpServer", nil)
		exposure := passwordExposure(fixture, agenticv1.GrantTypePassword)
		route := waitExposure(exposure)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			notReady := condition.NewNotReadyCondition(condition.ReasonValidationFailed, "Route is not ready")
			notReady.ObservedGeneration = route.Generation
			route.SetCondition(notReady)
			g.Expect(k8sClient.Status().Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			expectReadyReason(g, exposure, "ChildResourcesNotReady", metav1.ConditionFalse)
		}, timeout, interval).Should(Succeed())
		sub := passwordSubscription(fixture)
		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonPreconditionNotMet, metav1.ConditionFalse)
			g.Expect(sub.Status.ConsumeRoute).To(BeNil())
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
			fixture.expectNoProvisioning(g)
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	})

	DescribeTable("is blocked before approval without an explicit password grant", func(grantType agenticv1.GrantType) {
		f := newScopeFixture()
		f.server("McpServer", nil)
		waitExposure(passwordExposure(f, grantType))
		sub := passwordSubscription(f)

		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring(`grant type "password"`))
			f.expectNoProvisioning(g)
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	},
		Entry("client_credentials grant", agenticv1.GrantTypeClientCredentials),
		Entry("client_credentials grant", agenticv1.GrantTypeAuthorizationCode),
	)

	It("blocks Basic credentials with scopes without an external IDP", func() {
		f := newScopeFixture()
		f.server("McpServer", consumerScopes)
		waitExposure(f.exposure("exposure", consumerScopes, false))
		sub := passwordSubscription(f)

		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring(`grant type "password"`))
			f.expectNoProvisioning(g)
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	})
})
