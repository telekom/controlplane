// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		if grantType == agenticv1.GrantTypePassword {
			idp.Basic = &agenticv1.BasicAuthCredentials{Username: "provider-user", Password: "provider-pass"}
		}
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

	passwordSubscription := func(f *scopeFixture, scopes []string) *agenticv1.AgenticSubscription {
		sub := &agenticv1.AgenticSubscription{ObjectMeta: f.metadata("subscription"), Spec: agenticv1.AgenticSubscriptionSpec{
			BasePath: f.basePath, Zone: *ctypes.ObjectRefFromObject(f.zone),
			Requestor: agenticv1.Requestor{Application: *ctypes.ObjectRefFromObject(f.consumer)},
			Security: &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
				Client: &agenticv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"},
				Basic:  &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
				Scopes: scopes,
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
		sub := passwordSubscription(f, consumerScopes)
		grantApproval(sub, waitPending(sub))

		consume := waitConsumeRoute(sub, consumerScopes)
		Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
		Expect(consume.Spec.Security.M2M.Client).To(Equal(&gatewayv1.OAuth2ClientCredentials{
			ClientId: "consumer-client", ClientSecret: "consumer-secret",
		}))
	})

	It("does not create a password subscription while its exposure is not ready", func() {
		fixture := newScopeFixture()
		fixture.server("McpServer", nil)
		exposure := passwordExposure(fixture, "password")
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
		sub := passwordSubscription(fixture, consumerScopes)
		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonPreconditionNotMet, metav1.ConditionFalse)
			g.Expect(sub.Status.ConsumeRoute).To(BeNil())
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
			fixture.expectNoProvisioning(g)
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	})

	DescribeTable("blocks changes and preserves existing access when the exposure drops its external IDP", func(scopes []string) {
		f := newScopeFixture()
		f.server("McpServer", consumerScopes)
		exp := passwordExposure(f, agenticv1.GrantTypePassword)
		route := waitExposure(exp)
		sub := passwordSubscription(f, scopes)
		grantApproval(sub, waitPending(sub))
		consume := waitConsumeRoute(sub, scopes)
		acceptedConsume := consume.DeepCopy()
		acceptedRef := sub.Status.ConsumeRoute.DeepCopy()
		acceptedScopes := sub.Status.ActiveScopes

		var idp *agenticv1.ExternalIdentityProvider
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exp), exp)).To(Succeed())
			idp = exp.Spec.Security.M2M.ExternalIDP
			exp.Spec.Security.M2M = &agenticv1.Machine2MachineAuthentication{Scopes: consumerScopes}
			g.Expect(k8sClient.Update(ctx, exp)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeProcessing).Reason).To(Equal("Blocked"))
			g.Expect(sub.Status.ConsumeRoute).To(Equal(acceptedRef))
			g.Expect(sub.Status.ActiveScopes).To(Equal(acceptedScopes))
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consume), consume)).To(Succeed())
			g.Expect(consume.UID).To(Equal(acceptedConsume.UID))
			g.Expect(consume.Spec).To(Equal(acceptedConsume.Spec))
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), &gatewayv1.Route{})).To(Succeed())
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exp), exp)).To(Succeed())
			exp.Spec.Security.M2M = &agenticv1.Machine2MachineAuthentication{ExternalIDP: idp}
			g.Expect(k8sClient.Update(ctx, exp)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		waitExposure(exp)
		Expect(waitConsumeRoute(sub, scopes).UID).To(Equal(acceptedConsume.UID))
	},
		Entry("combined credentials without scopes", []string(nil)),
		Entry("combined credentials with scopes", consumerScopes),
	)

	DescribeTable("is provisioned for any exposure grant type and forwards the consumer grant type",
		func(exposureGrant, consumerGrant agenticv1.GrantType) {
			f := newScopeFixture()
			f.server("McpServer", nil)
			waitExposure(passwordExposure(f, exposureGrant))
			sub := &agenticv1.AgenticSubscription{ObjectMeta: f.metadata("subscription"), Spec: agenticv1.AgenticSubscriptionSpec{
				BasePath: f.basePath, Zone: *ctypes.ObjectRefFromObject(f.zone),
				Requestor: agenticv1.Requestor{Application: *ctypes.ObjectRefFromObject(f.consumer)},
				Security: &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
					Basic:     &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
					Scopes:    consumerScopes,
					GrantType: consumerGrant,
				}},
			}}
			Expect(k8sClient.Create(ctx, sub)).To(Succeed())
			grantApproval(sub, waitPending(sub))

			consume := waitConsumeRoute(sub, consumerScopes)
			Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
			Expect(consume.Spec.Security.M2M.GrantType).To(Equal(gatewayv1.GrantType(consumerGrant)))
		},
		Entry("client_credentials exposure grant overridden by consumer password grant", agenticv1.GrantTypeClientCredentials, agenticv1.GrantTypePassword),
		Entry("authorization_code exposure grant without consumer override", agenticv1.GrantTypeAuthorizationCode, agenticv1.GrantType("")),
	)

	DescribeTable("rejects provider username/password without the password grant",
		func(grantType agenticv1.GrantType) {
			f := newScopeFixture()
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

			err := k8sClient.Create(ctx, exp)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring(`Username/password credentials require grant type "password"`))
		},
		Entry("client_credentials grant", agenticv1.GrantTypeClientCredentials),
		Entry("omitted grant", agenticv1.GrantType("")),
	)

	DescribeTable("blocks consumer username/password without an external IDP", func(security *agenticv1.SubscriberMachine2MachineAuthentication) {
		f := newScopeFixture()
		f.server("McpServer", consumerScopes)
		waitExposure(f.exposure("exposure", consumerScopes, false))
		sub := &agenticv1.AgenticSubscription{ObjectMeta: f.metadata("subscription"), Spec: agenticv1.AgenticSubscriptionSpec{
			BasePath: f.basePath, Zone: *ctypes.ObjectRefFromObject(f.zone),
			Requestor: agenticv1.Requestor{Application: *ctypes.ObjectRefFromObject(f.consumer)},
			Security:  &agenticv1.SubscriberSecurity{M2M: security},
		}}
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())

		check := func(g Gomega) {
			expectReadyReason(g, sub, condition.ReasonValidationFailed, metav1.ConditionFalse)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring("requires an external IDP"))
			f.expectNoProvisioning(g)
		}
		Eventually(check, timeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	},
		Entry("combined credentials with scopes", &agenticv1.SubscriberMachine2MachineAuthentication{
			Client: &agenticv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"},
			Basic:  &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
			Scopes: consumerScopes,
		}),
		// The gateway never forwards username/password with a grant type as backend Basic auth.
		Entry("username/password with a grant type only", &agenticv1.SubscriberMachine2MachineAuthentication{
			Basic:     &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
			GrantType: agenticv1.GrantTypePassword,
		}),
	)
})
