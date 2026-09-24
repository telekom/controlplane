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
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	organizationapi "github.com/telekom/controlplane/organization/api/v1"

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
	Context("validateApiCategoryPolicy", func() {
		const (
			environment = "test"
			group       = "alpha"
			teamName    = "core"
		)

		baseApp := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "consumer-app",
				Namespace: environment + "--" + group + "--" + teamName,
			},
		}
		baseAPI := &apiv1.Api{
			Spec: apiv1.ApiSpec{
				Category: "partner",
			},
		}

		type testCase struct {
			name           string
			teamCategory   organizationapi.TeamCategory
			apiCategories  []apiv1.ApiCategory
			expectedResult bool
			expectedReason string
		}

		tests := []testCase{
			{
				name:         "allowed category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiv1.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiv1.ApiCategorySpec{
							LabelValue: "partner",
							Active:     true,
							AllowTeams: &apiv1.AllowTeamsConfig{Categories: []string{"Customer"}},
						},
					},
				},
				expectedResult: true,
			},
			{
				name:         "denied category",
				teamCategory: organizationapi.TeamCategoryInfrastructure,
				apiCategories: []apiv1.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiv1.ApiCategorySpec{
							LabelValue: "partner",
							Active:     true,
							AllowTeams: &apiv1.AllowTeamsConfig{Categories: []string{"Customer"}},
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryTeamCategoryNotAllowedReason,
			},
			{
				name:           "no categories configured",
				teamCategory:   organizationapi.TeamCategoryCustomer,
				apiCategories:  nil,
				expectedResult: true,
			},
			{
				name:         "missing category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiv1.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiv1.ApiCategorySpec{
							LabelValue: "other",
							Active:     true,
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryPolicyResolutionFailedReason,
			},
			{
				name:         "inactive category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiv1.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiv1.ApiCategorySpec{
							LabelValue: "partner",
							Active:     false,
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryPolicyResolutionFailedReason,
			},
		}

		for _, tt := range tests {
			It(tt.name, func() {
				team := &organizationapi.Team{
					ObjectMeta: metav1.ObjectMeta{
						Name:      group + "--" + teamName,
						Namespace: environment,
						Labels: map[string]string{
							config.EnvironmentLabelKey: environment,
						},
					},
					Spec: organizationapi.TeamSpec{
						Group:    group,
						Name:     teamName,
						Email:    "team@example.com",
						Category: tt.teamCategory,
					},
				}

				objects := []crclient.Object{team}
				for i := range tt.apiCategories {
					cat := tt.apiCategories[i]
					objects = append(objects, &cat)
				}

				ctx := newClientContext(environment, objects...)
				apiSub := &apiv1.ApiSubscription{}

				result := validateApiCategoryPolicy(ctx, baseAPI, baseApp, apiSub)
				Expect(result).To(Equal(tt.expectedResult))

				if tt.expectedReason == "" {
					notReady := meta.FindStatusCondition(apiSub.GetConditions(), condition.ConditionTypeReady)
					Expect(notReady == nil || notReady.Status != metav1.ConditionFalse).To(BeTrue())
					return
				}

				notReady := meta.FindStatusCondition(apiSub.GetConditions(), condition.ConditionTypeReady)
				Expect(notReady).NotTo(BeNil())
				Expect(notReady.Reason).To(Equal(tt.expectedReason))
			})
		}
	})
})

func newClientContext(environment string, objects ...crclient.Object) context.Context {
	sch := runtime.NewScheme()
	Expect(apiv1.AddToScheme(sch)).To(Succeed())
	Expect(applicationapi.AddToScheme(sch)).To(Succeed())
	Expect(organizationapi.AddToScheme(sch)).To(Succeed())
	fakeClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(objects...).
		WithIndex(&apiv1.ApiCategory{}, "spec.labelValue", func(obj crclient.Object) []string {
			apiCategory, ok := obj.(*apiv1.ApiCategory)
			if !ok || apiCategory.Spec.LabelValue == "" {
				return nil
			}
			return []string{apiCategory.Spec.LabelValue}
		}).
		Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}
