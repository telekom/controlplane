// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consumer username/password with scopes", func() {
	consumerScopes := []string{"consumer:read", "consumer:write"}

	withConsumerPassword := func(sub *apiapi.ApiSubscription) {
		sub.Spec.Security.M2M.Client = &apiapi.OAuth2ClientCredentials{
			ClientId: "consumer-client", ClientSecret: "consumer-secret",
		}
		sub.Spec.Security.M2M.Basic = &apiapi.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}
		sub.Spec.Security.M2M.Scopes = consumerScopes
	}

	It("is provisioned when the exposure uses an external IDP password grant", func() {
		exposure, sub := newScopeFixtures("password-grant", nil, true)
		idp := exposure.Spec.Security.M2M.ExternalIDP
		idp.GrantType = apiapi.GrantTypePassword
		idp.Client = nil
		idp.Basic = &apiapi.BasicAuthCredentials{Username: "provider-user", Password: "provider-pass"}
		withConsumerPassword(sub)

		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		Eventually(func(g Gomega) { expectScopePending(g, sub) }, scopeTimeout, interval).Should(Succeed())

		request := ProgressApprovalRequest(sub.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sub, approvalapi.ApprovalStateGranted, request)
		CompleteSubscriptionChildren(sub)
		Eventually(func(g Gomega) {
			expectScopeRoutes(g, exposure, sub)
			consume := &gatewayapi.ConsumeRoute{}
			g.Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
			g.Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayapi.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
			g.Expect(consume.Spec.Security.M2M.Client).To(Equal(&gatewayapi.OAuth2ClientCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret",
			}))
		}, scopeTimeout, interval).Should(Succeed())
	})

	DescribeTable("blocks changes and preserves existing access when the exposure drops its external IDP", func(name string, scopes []string) {
		// Declared scopes keep the specification-scope check out of the way once the external IDP is removed.
		exposure, sub := newScopeFixtures(name, consumerScopes, true)
		exposure.Spec.Security.M2M.ExternalIDP.GrantType = apiapi.GrantTypePassword
		withConsumerPassword(sub)
		sub.Spec.Security.M2M.Scopes = scopes
		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			g.Expect(sub.Status.ApprovalRequest).NotTo(BeNil())
		}, scopeTimeout, interval).Should(Succeed())
		request := ProgressApprovalRequest(sub.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sub, approvalapi.ApprovalStateGranted, request)
		CompleteSubscriptionChildren(sub)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionTrue, condition.ReasonProvisioned, sub.Generation)
		}, scopeTimeout, interval).Should(Succeed())
		consumeRef := *sub.Status.ConsumeRoute
		routeRef := *sub.Status.Route
		failover := &gatewayapi.ConsumeRoute{}
		Expect(k8sClient.Get(ctx, consumeRef.K8s(), failover)).To(Succeed())
		acceptedConsume := failover.DeepCopy()
		acceptedScopes := sub.Status.ActiveScopes
		acceptedFailoverRefs := sub.Status.FailoverConsumeRoutes
		failover.ObjectMeta = metav1.ObjectMeta{Name: failover.Name + "-failover", Namespace: failover.Namespace, Labels: failover.Labels}
		Expect(controllerutil.SetControllerReference(sub, failover, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, failover)).To(Succeed())

		By("Removing the exposure external IDP")
		var idp *apiapi.ExternalIdentityProvider
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exposure), exposure)).To(Succeed())
			idp = exposure.Spec.Security.M2M.ExternalIDP
			exposure.Spec.Security.M2M = &apiapi.Machine2MachineAuthentication{Scopes: consumerScopes}
			g.Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		}, scopeTimeout, interval).Should(Succeed())
		check := func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonValidationFailed, sub.Generation)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeProcessing).Reason).To(Equal("Blocked"))
			g.Expect(sub.Status.ConsumeRoute).To(Equal(&consumeRef))
			g.Expect(sub.Status.FailoverConsumeRoutes).To(Equal(acceptedFailoverRefs))
			g.Expect(sub.Status.ActiveScopes).To(Equal(acceptedScopes))
			for _, accepted := range []*gatewayapi.ConsumeRoute{acceptedConsume, failover} {
				current := &gatewayapi.ConsumeRoute{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(accepted), current)).To(Succeed())
				g.Expect(current.UID).To(Equal(accepted.UID))
				g.Expect(current.Spec).To(Equal(accepted.Spec))
			}
			g.Expect(k8sClient.Get(ctx, routeRef.K8s(), &gatewayapi.Route{})).To(Succeed())
		}
		Eventually(check, scopeTimeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())

		By("Restoring the external IDP")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(exposure), exposure)).To(Succeed())
			exposure.Spec.Security.M2M = &apiapi.Machine2MachineAuthentication{ExternalIDP: idp}
			g.Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
		}, scopeTimeout, interval).Should(Succeed())
		CompleteSubscriptionChildren(sub)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionTrue, condition.ReasonProvisioned, sub.Generation)
			g.Expect(sub.Status.ConsumeRoute).To(Equal(&consumeRef))
			current := &gatewayapi.ConsumeRoute{}
			g.Expect(k8sClient.Get(ctx, consumeRef.K8s(), current)).To(Succeed())
			g.Expect(current.UID).To(Equal(acceptedConsume.UID))
		}, scopeTimeout, interval).Should(Succeed())
	},
		Entry("combined credentials without scopes", "password-revoke-no-scopes", nil),
		Entry("combined credentials with scopes", "password-revoke-scopes", consumerScopes),
	)

	DescribeTable("is provisioned for any exposure grant type and forwards the consumer grant type",
		func(name string, exposureGrant, consumerGrant apiapi.GrantType, consumerTokenRequest apiapi.TokenRequestMethod) {
			exposure, sub := newScopeFixtures(name, nil, true)
			exposure.Spec.Security.M2M.ExternalIDP.GrantType = exposureGrant
			withConsumerPassword(sub)
			sub.Spec.Security.M2M.GrantType = consumerGrant
			sub.Spec.Security.M2M.TokenRequest = consumerTokenRequest

			Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
			Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
			Expect(k8sClient.Create(ctx, sub)).To(Succeed())
			Eventually(func(g Gomega) { expectScopePending(g, sub) }, scopeTimeout, interval).Should(Succeed())

			request := ProgressApprovalRequest(sub.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(sub, approvalapi.ApprovalStateGranted, request)
			CompleteSubscriptionChildren(sub)
			Eventually(func(g Gomega) {
				expectScopeRoutes(g, exposure, sub)
				consume := &gatewayapi.ConsumeRoute{}
				g.Expect(k8sClient.Get(ctx, sub.Status.ConsumeRoute.K8s(), consume)).To(Succeed())
				g.Expect(consume.Spec.Security.M2M.Basic).To(Equal(&gatewayapi.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}))
				g.Expect(consume.Spec.Security.M2M.Client).To(Equal(&gatewayapi.OAuth2ClientCredentials{
					ClientId: "consumer-client", ClientSecret: "consumer-secret",
				}))
				g.Expect(consume.Spec.Security.M2M.Scopes).To(Equal(consumerScopes))
				g.Expect(consume.Spec.Security.M2M.GrantType).To(Equal(gatewayapi.GrantType(consumerGrant)))
				g.Expect(consume.Spec.Security.M2M.TokenRequest).To(Equal(gatewayapi.TokenRequestMethod(consumerTokenRequest)))
			}, scopeTimeout, interval).Should(Succeed())
		},
		Entry("client_credentials exposure grant overridden by consumer password grant",
			"password-override", apiapi.GrantTypeClientCredentials, apiapi.GrantTypePassword, apiapi.TokenRequestMethod("")),
		Entry("authorization_code exposure grant without consumer override",
			"password-no-override", apiapi.GrantTypeAuthorizationCode, apiapi.GrantType(""), apiapi.TokenRequestMethod("")),
		Entry("consumer token request method overrides the exposure method",
			"password-token-request", apiapi.GrantTypeClientCredentials, apiapi.GrantTypePassword, apiapi.TokenRequestClientSecretPost),
	)

	DescribeTable("is blocked before approval when the exposure has no external IDP", func(name string, security func(*apiapi.ApiSubscription)) {
		// Declared scopes keep the specification-scope check out of the way.
		exposure, sub := newScopeFixtures(name, consumerScopes, true)
		exposure.Spec.Security.M2M.ExternalIDP = nil
		exposure.Spec.Security.M2M.Scopes = consumerScopes
		security(sub)

		Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		Eventually(func(g Gomega) { expectScopeExposureReady(g, exposure) }, scopeTimeout, interval).Should(Succeed())
		Expect(k8sClient.Create(ctx, sub)).To(Succeed())

		check := func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), sub)).To(Succeed())
			expectScopeCondition(g, sub.GetConditions(), condition.ConditionTypeReady, metav1.ConditionFalse, condition.ReasonValidationFailed, sub.Generation)
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Message).To(ContainSubstring("requires an external IDP"))
			g.Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeProcessing).Reason).To(Equal("Blocked"))
			g.Expect(sub.Status.ApprovalRequest).To(BeNil())
			g.Expect(sub.Status.ConsumeRoute).To(BeNil())
		}
		Eventually(check, scopeTimeout, interval).Should(Succeed())
		Consistently(check, time.Second, interval).Should(Succeed())
	},
		Entry("combined credentials with scopes", "password-no-external-idp", withConsumerPassword),
		// The gateway never forwards username/password with a grant type as backend Basic auth.
		Entry("username/password with a grant type only", "password-grant-no-external-idp", func(sub *apiapi.ApiSubscription) {
			sub.Spec.Security.M2M = &apiapi.SubscriberMachine2MachineAuthentication{
				Basic:     &apiapi.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
				GrantType: apiapi.GrantTypePassword,
			}
		}),
	)
})
