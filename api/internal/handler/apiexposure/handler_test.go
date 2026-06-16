// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"testing"

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
)

func TestValidateApiCategoryPolicy(t *testing.T) {
	t.Parallel()

	const (
		environment = "test"
		group       = "alpha"
		teamName    = "core"
	)

	baseApp := &applicationapi.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-app",
			Namespace: environment + "--" + group + "--" + teamName,
		},
	}
	baseAPI := &apiv1.Api{
		Spec: apiv1.ApiSpec{
			Category: "partner",
		},
	}

	tests := []struct {
		name           string
		teamCategory   organizationapi.TeamCategory
		apiCategories  []apiv1.ApiCategory
		expectedResult bool
		expectedReason string
	}{
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
			name:           "unresolved category",
			teamCategory:   organizationapi.TeamCategoryCustomer,
			apiCategories:  nil,
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

			ctx := newClientContext(t, environment, objects...)
			apiExp := &apiv1.ApiExposure{}

			result := validateApiCategoryPolicy(ctx, baseAPI, baseApp, apiExp)
			if result != tt.expectedResult {
				t.Fatalf("expected result %v, got %v", tt.expectedResult, result)
			}

			if tt.expectedReason == "" {
				notReady := meta.FindStatusCondition(apiExp.GetConditions(), condition.ConditionTypeReady)
				if notReady != nil && notReady.Status == metav1.ConditionFalse {
					t.Fatalf("expected no not-ready condition for allowed path, got: %#v", notReady)
				}
				return
			}

			notReady := meta.FindStatusCondition(apiExp.GetConditions(), condition.ConditionTypeReady)
			if notReady == nil {
				t.Fatalf("expected not-ready condition to be set")
			}
			if notReady.Reason != tt.expectedReason {
				t.Fatalf("expected reason %q, got %q", tt.expectedReason, notReady.Reason)
			}
		})
	}
}

func newClientContext(t *testing.T, environment string, objects ...crclient.Object) context.Context {
	t.Helper()

	sch := runtime.NewScheme()
	if err := apiv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register api scheme: %v", err)
	}
	if err := applicationapi.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register application scheme: %v", err)
	}
	if err := organizationapi.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register organization scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(objects...).Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}
