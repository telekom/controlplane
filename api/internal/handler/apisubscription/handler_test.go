// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription scope validation", func() {
	endpoint := &apiv1.ExternalIdentityProvider{TokenEndpoint: "https://idp.example/token"}
	emptyEndpoint := &apiv1.ExternalIdentityProvider{}
	requested := []string{"consumer:z", "consumer:a", "consumer:z"}

	DescribeTable("preserves scope decisions and approval properties", func(idp *apiv1.ExternalIdentityProvider, declared, scopes []string, allowed bool) {
		api := &apiv1.Api{Spec: apiv1.ApiSpec{Oauth2Scopes: declared}}
		exposure := &apiv1.ApiExposure{Spec: apiv1.ApiExposureSpec{Security: &apiv1.Security{
			M2M: &apiv1.Machine2MachineAuthentication{ExternalIDP: idp, Scopes: []string{"provider:only"}},
		}}}
		sub := &apiv1.ApiSubscription{Spec: apiv1.ApiSubscriptionSpec{Security: &apiv1.SubscriberSecurity{
			M2M: &apiv1.SubscriberMachine2MachineAuthentication{Scopes: scopes},
		}}}
		original := sub.DeepCopy().Spec
		properties := map[string]any{"resource_type": "API", "basePath": "/scope/test"}
		Expect(validateSubscriptionScopes(context.Background(), api, exposure, sub, properties)).To(Equal(allowed))
		Expect(sub.Spec).To(Equal(original))
		Expect(properties).To(HaveKeyWithValue("resource_type", "API"))
		Expect(properties).To(HaveKeyWithValue("basePath", "/scope/test"))
		scopeCondition := meta.FindStatusCondition(sub.GetConditions(), "ScopesAllowed")
		if scopes == nil {
			Expect(scopeCondition).To(BeNil())
			Expect(properties).NotTo(HaveKey("scopes"))
			return
		}
		Expect(scopeCondition).NotTo(BeNil())
		if allowed {
			Expect(scopeCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(scopeCondition.Reason).To(Equal("Allowed"))
			Expect(properties).To(HaveKeyWithValue("scopes", scopes))
			return
		}
		Expect(properties).NotTo(HaveKey("scopes"))
		Expect(scopeCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(scopeCondition.Reason).To(Equal("NotAllowed"))
		ready := meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady)
		blocked := meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeProcessing)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(condition.ReasonValidationFailed))
		Expect(blocked).NotTo(BeNil())
		Expect(blocked.Reason).To(Equal("Blocked"))
		if len(declared) == 0 {
			Expect(ready.Message).To(Equal("Api does not define any Oauth2 scopes"))
			Expect(blocked.Message).To(Equal("Api does not define any Oauth2 scopes. ApiSubscription will be automatically processed, if the API will be updated with scopes"))
		} else {
			Expect(ready.Message).To(Equal("One or more scopes which are defined in ApiSubscription are not defined in the ApiSpecification"))
			Expect(blocked.Message).To(Equal(fmt.Sprintf("Some defined scopes are not available. Available scopes: %q. Unsupported scopes: %q", strings.Join(declared, ", "), strings.Join(scopes, ", "))))
		}
	},
		Entry("external endpoint with no declared scopes", endpoint, nil, requested, true),
		Entry("external endpoint with unmatched scopes", endpoint, []string{"spec:only"}, requested, true),
		Entry("absent endpoint with no declared scopes", nil, nil, requested, false),
		Entry("empty endpoint with no declared scopes", emptyEndpoint, nil, requested, false),
		Entry("absent endpoint with unmatched scopes", nil, []string{"spec:only"}, requested, false),
		Entry("empty endpoint with unmatched scopes", emptyEndpoint, []string{"spec:only"}, requested, false),
		Entry("ordinary valid subset", nil, []string{"consumer:a", "consumer:z", "extra"}, requested, true),
		Entry("nil scopes without an endpoint", nil, nil, nil, true),
		Entry("nil scopes with an endpoint", endpoint, nil, nil, true),
		Entry("empty scopes without declarations still fail", nil, nil, []string{}, false),
		Entry("empty scopes with declarations succeed", emptyEndpoint, []string{"spec:only"}, []string{}, true),
		Entry("empty exempt scopes are recorded", endpoint, nil, []string{}, true),
	)

	It("replaces a prior false scope condition when the exposure becomes exempt", func() {
		api := &apiv1.Api{}
		exposure := &apiv1.ApiExposure{}
		sub := &apiv1.ApiSubscription{Spec: apiv1.ApiSubscriptionSpec{Security: &apiv1.SubscriberSecurity{
			M2M: &apiv1.SubscriberMachine2MachineAuthentication{Scopes: requested},
		}}}
		properties := map[string]any{"resource_type": "API"}
		Expect(validateSubscriptionScopes(context.Background(), api, exposure, sub, properties)).To(BeFalse())
		exposure.Spec.Security = &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{ExternalIDP: endpoint}}
		Expect(validateSubscriptionScopes(context.Background(), api, exposure, sub, properties)).To(BeTrue())
		Expect(meta.FindStatusCondition(sub.GetConditions(), "ScopesAllowed").Status).To(Equal(metav1.ConditionTrue))
		Expect(properties).To(Equal(map[string]any{"resource_type": "API", "scopes": requested}))
	})

	It("does not record scopes without M2M security", func() {
		for _, security := range []*apiv1.SubscriberSecurity{nil, {}} {
			sub := &apiv1.ApiSubscription{Spec: apiv1.ApiSubscriptionSpec{Security: security}}
			properties := map[string]any{"resource_type": "API"}
			Expect(validateSubscriptionScopes(context.Background(), &apiv1.Api{}, &apiv1.ApiExposure{}, sub, properties)).To(BeTrue())
			Expect(sub.GetConditions()).To(BeEmpty())
			Expect(properties).To(Equal(map[string]any{"resource_type": "API"}))
		}
	})
})

var _ = Describe("ApiSubscription Handler", func() {
	Context("validateBasicWithScopesPolicy", func() {
		basic := &apiv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"}
		withBasicAndScopes := &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
			Basic: basic, Scopes: []string{"consumer:read"},
		}}
		exposureWithGrant := func(grantType apiv1.GrantType) *apiv1.ApiExposure {
			return &apiv1.ApiExposure{Spec: apiv1.ApiExposureSpec{Security: &apiv1.Security{
				M2M: &apiv1.Machine2MachineAuthentication{ExternalIDP: &apiv1.ExternalIdentityProvider{
					TokenEndpoint: "https://idp.example/token", GrantType: grantType,
				}},
			}}}
		}

		DescribeTable("returns a validation error only for incompatible Basic credentials with scopes",
			func(security *apiv1.SubscriberSecurity, exposure *apiv1.ApiExposure, blocked bool) {
				sub := &apiv1.ApiSubscription{Spec: apiv1.ApiSubscriptionSpec{Security: security}}
				err := validateBasicWithScopesPolicy(sub, exposure)
				if !blocked {
					Expect(err).NotTo(HaveOccurred())
					return
				}
				Expect(err).To(MatchError(`Consumer username/password with scopes requires an external IDP grant type "password"`))
			},
			Entry("without subscription security", nil, nil, false),
			Entry("without subscription M2M", &apiv1.SubscriberSecurity{}, nil, false),
			Entry("with scopes only", &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Scopes: []string{"consumer:read"},
			}}, nil, false),
			Entry("with Basic only", &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Basic: basic,
			}}, nil, false),
			Entry("with empty scopes", &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{
				Basic: basic, Scopes: []string{},
			}}, nil, false),
			Entry("without an exposure", withBasicAndScopes, nil, true),
			Entry("without exposure security", withBasicAndScopes, &apiv1.ApiExposure{}, true),
			Entry("without exposure M2M", withBasicAndScopes, &apiv1.ApiExposure{Spec: apiv1.ApiExposureSpec{
				Security: &apiv1.Security{},
			}}, true),
			Entry("without an external IDP", withBasicAndScopes, &apiv1.ApiExposure{Spec: apiv1.ApiExposureSpec{
				Security: &apiv1.Security{M2M: &apiv1.Machine2MachineAuthentication{}},
			}}, true),
			Entry("with an omitted grant", withBasicAndScopes, exposureWithGrant(""), true),
			Entry("with a client_credentials grant", withBasicAndScopes, exposureWithGrant(apiv1.GrantTypeClientCredentials), true),
			Entry("with an authorization_code grant", withBasicAndScopes, exposureWithGrant(apiv1.GrantTypeAuthorizationCode), true),
			Entry("with a password grant", withBasicAndScopes, exposureWithGrant(apiv1.GrantTypePassword), false),
		)
	})
})
