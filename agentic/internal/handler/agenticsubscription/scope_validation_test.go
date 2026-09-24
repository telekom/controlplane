// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import (
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription scope validation", func() {
	exposure := func(endpoint string) *agenticv1.AgenticExposure {
		return &agenticv1.AgenticExposure{Spec: agenticv1.AgenticExposureSpec{
			Security: &agenticv1.Security{M2M: &agenticv1.Machine2MachineAuthentication{
				ExternalIDP: &agenticv1.ExternalIdentityProvider{TokenEndpoint: endpoint}, Scopes: []string{"provider"},
			}},
		}}
	}
	DescribeTable("uses only the selected exposure's token endpoint",
		func(exp *agenticv1.AgenticExposure, declared, requested []string, accepted bool, reason string) {
			obj := &agenticv1.AgenticSubscription{Spec: agenticv1.AgenticSubscriptionSpec{
				Security: &agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{Scopes: requested}},
			}}
			server := &util.ServerInfo{Oauth2Scopes: declared}
			before, serverBefore := slices.Clone(requested), slices.Clone(declared)
			exposureBefore := exp.DeepCopy()
			Expect(validateSubscriptionScopes(server, exp, obj)).To(Equal(accepted))
			Expect(obj.Spec.Security.M2M.Scopes).To(Equal(before))
			Expect(server.Oauth2Scopes).To(Equal(serverBefore))
			Expect(exp).To(Equal(exposureBefore))
			if !accepted {
				ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
				Expect(ready).NotTo(BeNil())
				Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				Expect(ready.Reason).To(Equal(reason))
				Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing).Reason).To(Equal("Blocked"))
			}
		},
		Entry("endpoint without declared scopes", exposure("https://idp.example/token"), nil, []string{"z", "a", "z"}, true, ""),
		Entry("endpoint with unmatched server and exposure scopes", exposure("https://idp.example/token"), []string{"read"}, []string{"z", "a", "z"}, true, ""),
		Entry("nil exposure without declarations", nil, nil, []string{"read"}, false, "ScopesNotDefined"),
		Entry("nil exposure with unmatched scopes", nil, []string{"read"}, []string{"write"}, false, "InvalidScopes"),
		Entry("absent endpoint", &agenticv1.AgenticExposure{}, []string{"read"}, []string{"write"}, false, "InvalidScopes"),
		Entry("empty endpoint without declarations", exposure(""), nil, []string{"read"}, false, "ScopesNotDefined"),
		Entry("empty endpoint with unmatched scopes", exposure(""), []string{"read"}, []string{"write"}, false, "InvalidScopes"),
		Entry("valid subset", exposure(""), []string{"read", "write"}, []string{"write", "read"}, true, ""),
		Entry("nil scopes", nil, nil, nil, true, ""),
		Entry("empty scopes without declarations", nil, nil, []string{}, false, "ScopesNotDefined"),
		Entry("empty subset", nil, []string{"read"}, []string{}, true, ""),
		Entry("exempt empty scopes", exposure("https://idp.example/token"), nil, []string{}, true, ""),
	)
	It("accepts absent security or M2M", func() {
		for _, security := range []*agenticv1.SubscriberSecurity{nil, {}} {
			obj := &agenticv1.AgenticSubscription{Spec: agenticv1.AgenticSubscriptionSpec{Security: security}}
			Expect(validateSubscriptionScopes(&util.ServerInfo{}, nil, obj)).To(BeTrue())
		}
	})
})
