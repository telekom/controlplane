// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consumer username/password with scopes", func() {
	consumerScopes := []string{"consumer.read", "consumer.write"}

	passwordExposure := func(f *scopeFixture, grantType string) *agenticv1.AgenticExposure {
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

	DescribeTable("is blocked before approval without an explicit password grant", func(grantType string) {
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
		Entry("client_credentials grant", "client_credentials"),
		Entry("omitted grant type keeps its legacy meaning", ""),
	)
})
