// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
)

func TestResolveActiveApiCategoryByLabelValue(t *testing.T) {
	t.Parallel()

	const environment = "test"
	activeCategory := &apiv1.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partner",
			Namespace: environment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: environment,
			},
		},
		Spec: apiv1.ApiCategorySpec{
			LabelValue: "partner",
			Active:     true,
		},
	}
	inactiveCategory := &apiv1.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy",
			Namespace: environment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: environment,
			},
		},
		Spec: apiv1.ApiCategorySpec{
			LabelValue: "legacy",
			Active:     false,
		},
	}

	ctx := newClientContext(t, environment, activeCategory, inactiveCategory)

	if _, err := ResolveActiveApiCategoryByLabelValue(ctx, "partner"); err != nil {
		t.Fatalf("expected active category to resolve, got error: %v", err)
	}
	if _, err := ResolveActiveApiCategoryByLabelValue(ctx, "legacy"); err == nil {
		t.Fatalf("expected inactive category to fail resolution")
	}
	if _, err := ResolveActiveApiCategoryByLabelValue(ctx, "missing"); err == nil {
		t.Fatalf("expected missing category to fail resolution")
	}
}

func TestResolveActiveApiCategoryForApi(t *testing.T) {
	t.Parallel()

	const environment = "test"
	category := &apiv1.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal",
			Namespace: environment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: environment,
			},
		},
		Spec: apiv1.ApiCategorySpec{
			LabelValue: "internal",
			Active:     true,
		},
	}
	api := &apiv1.Api{
		Spec: apiv1.ApiSpec{
			Category: "internal",
		},
	}

	ctx := newClientContext(t, environment, category)
	if _, err := ResolveActiveApiCategoryForApi(ctx, api); err != nil {
		t.Fatalf("expected api category resolution to succeed, got error: %v", err)
	}
}

func newClientContext(t *testing.T, environment string, objects ...crclient.Object) context.Context {
	t.Helper()

	sch := runtime.NewScheme()
	if err := apiv1.AddToScheme(sch); err != nil {
		t.Fatalf("failed to register api scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(objects...).Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}
