// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription approval scope snapshots", func() {
	var (
		obj      *agenticv1.AgenticSubscription
		exposure *agenticv1.AgenticExposure
	)

	BeforeEach(func() {
		obj = &agenticv1.AgenticSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "scope-subscription"},
			Spec:       agenticv1.AgenticSubscriptionSpec{BasePath: "/mcp/weather/v1"},
		}
		exposure = &agenticv1.AgenticExposure{
			Spec: agenticv1.AgenticExposureSpec{Variant: agenticv1.AgenticVariantMCP},
		}
	})

	requesterFor := func() approvalv1.Requester {
		requester := approvalv1.Requester{}
		Expect(requester.SetProperties(subscriptionApprovalProperties(obj, exposure))).To(Succeed())
		return requester
	}

	It("includes scopes in the serialized snapshot and changes request identity when scopes change", func() {
		obj.Spec.Security = &agenticv1.SubscriberSecurity{
			M2M: &agenticv1.SubscriberMachine2MachineAuthentication{Scopes: []string{"read"}},
		}
		original := requesterFor()
		originalName := approvalv1.ApprovalRequestName(obj, original.Properties)

		var properties map[string]any
		Expect(json.Unmarshal(original.Properties.Raw, &properties)).To(Succeed())
		Expect(properties).To(HaveKeyWithValue("scopes", []any{"read"}))
		Expect(properties).To(HaveKeyWithValue("basePath", obj.Spec.BasePath))
		Expect(properties).To(HaveKeyWithValue("resource_name", obj.Spec.BasePath))
		Expect(properties).To(HaveKeyWithValue("resource_type", exposure.Spec.Variant.DisplayType()))

		unchanged := requesterFor()
		Expect(approvalv1.ApprovalRequestName(obj, unchanged.Properties)).To(Equal(originalName))

		obj.Spec.Security.M2M.Scopes = []string{"read", "write"}
		expanded := requesterFor()
		Expect(approvalv1.ApprovalRequestName(obj, expanded.Properties)).NotTo(Equal(originalName))

		obj.Spec.Security.M2M.Scopes = nil
		removed := requesterFor()
		Expect(approvalv1.ApprovalRequestName(obj, removed.Properties)).NotTo(Equal(originalName))
		Expect(approvalv1.ApprovalRequestName(obj, removed.Properties)).NotTo(Equal(approvalv1.ApprovalRequestName(obj, expanded.Properties)))
	})

	DescribeTable("matches API scope-property presence",
		func(security *agenticv1.SubscriberSecurity, present bool) {
			obj.Spec.Security = security
			properties := subscriptionApprovalProperties(obj, exposure)
			if present {
				Expect(properties).To(HaveKeyWithValue("scopes", []string{}))
			} else {
				Expect(properties).NotTo(HaveKey("scopes"))
			}
		},
		Entry("without security", nil, false),
		Entry("without M2M", &agenticv1.SubscriberSecurity{}, false),
		Entry("with nil scopes", &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{}}, false),
		Entry("with explicit empty scopes", &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{Scopes: []string{}}}, true),
	)
})
