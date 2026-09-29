// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Exposure scope validation", func() {
	DescribeTable("preserves specification checks except with a token endpoint",
		func(idp *agenticv1.ExternalIdentityProvider, declared, requested []string, accepted bool, reason string) {
			obj := &agenticv1.AgenticExposure{Spec: agenticv1.AgenticExposureSpec{
				Security: &agenticv1.Security{M2M: &agenticv1.Machine2MachineAuthentication{ExternalIDP: idp, Scopes: requested}},
			}}
			server := &util.ServerInfo{Oauth2Scopes: declared}
			before, serverBefore := slices.Clone(requested), slices.Clone(declared)
			Expect(validateExposureScopes(context.Background(), server, obj)).To(Equal(accepted))
			Expect(obj.Spec.Security.M2M.Scopes).To(Equal(before))
			Expect(server.Oauth2Scopes).To(Equal(serverBefore))
			if !accepted {
				ready := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
				Expect(ready).NotTo(BeNil())
				Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				Expect(ready.Reason).To(Equal(reason))
				Expect(meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing).Reason).To(Equal("Blocked"))
			}
		},
		Entry("endpoint without declared scopes", &agenticv1.ExternalIdentityProvider{TokenEndpoint: "https://idp.example/token"}, nil, []string{"z", "a", "z"}, true, ""),
		Entry("endpoint with unmatched scopes", &agenticv1.ExternalIdentityProvider{TokenEndpoint: "https://idp.example/token"}, []string{"read"}, []string{"z", "a", "z"}, true, ""),
		Entry("no endpoint and no declarations", nil, nil, []string{"read"}, false, "ScopesNotDefined"),
		Entry("empty endpoint and no declarations", &agenticv1.ExternalIdentityProvider{}, nil, []string{"read"}, false, "ScopesNotDefined"),
		Entry("empty endpoint and unmatched scopes", &agenticv1.ExternalIdentityProvider{}, []string{"read"}, []string{"write"}, false, "InvalidScopes"),
		Entry("no endpoint and unmatched scopes", nil, []string{"read"}, []string{"write"}, false, "InvalidScopes"),
		Entry("valid subset", nil, []string{"read", "write"}, []string{"write", "read"}, true, ""),
		Entry("nil scopes", nil, nil, nil, true, ""),
		Entry("empty scopes without declarations", nil, nil, []string{}, false, "ScopesNotDefined"),
		Entry("empty subset", nil, []string{"read"}, []string{}, true, ""),
		Entry("exempt empty scopes", &agenticv1.ExternalIdentityProvider{TokenEndpoint: "https://idp.example/token"}, nil, []string{}, true, ""),
	)
	It("accepts absent security or M2M", func() {
		for _, security := range []*agenticv1.Security{nil, {}} {
			obj := &agenticv1.AgenticExposure{Spec: agenticv1.AgenticExposureSpec{Security: security}}
			Expect(validateExposureScopes(context.Background(), &util.ServerInfo{}, obj)).To(BeTrue())
		}
	})
})
