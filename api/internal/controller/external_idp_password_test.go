// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		sub.Spec.Security.M2M.Client = nil
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
			g.Expect(consume.Spec.Security.M2M.Client).To(BeNil())
		}, scopeTimeout, interval).Should(Succeed())
	})

	It("is blocked before approval when the exposure has no external IDP", func() {
		// Declared scopes keep the specification-scope check out of the way.
		exposure, sub := newScopeFixtures("password-no-external-idp", consumerScopes, true)
		exposure.Spec.Security.M2M.ExternalIDP = nil
		exposure.Spec.Security.M2M.Scopes = consumerScopes
		withConsumerPassword(sub)

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
	})
})
