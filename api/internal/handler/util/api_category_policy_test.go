// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiCategory Policy", func() {
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

	DescribeTable("ResolveActiveApiCategoryByLabelValue",
		func(objects []crclient.Object, labelValue string, expectNil, expectErr bool) {
			ctx := newClientContext(environment, objects...)
			category, err := ResolveActiveApiCategoryByLabelValue(ctx, labelValue)

			if expectErr {
				Expect(err).To(HaveOccurred())
				return
			}

			Expect(err).NotTo(HaveOccurred())
			if expectNil {
				Expect(category).To(BeNil())
				return
			}

			Expect(category).NotTo(BeNil())
			Expect(category.Spec.LabelValue).To(Equal(labelValue))
		},
		Entry("resolves an active category", []crclient.Object{activeCategory}, "partner", false, false),
		Entry("skips validation when no categories exist", []crclient.Object{}, "partner", true, false),
		Entry("rejects an inactive category", []crclient.Object{inactiveCategory}, "legacy", true, true),
		Entry("rejects a missing category when categories exist", []crclient.Object{activeCategory}, "missing", true, true),
	)

	Describe("ResolveActiveApiCategoryForApi", func() {
		It("resolves the category from the api spec", func() {
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

			ctx := newClientContext(environment, category)
			resolved, err := ResolveActiveApiCategoryForApi(ctx, api)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).NotTo(BeNil())
			Expect(resolved.Spec.LabelValue).To(Equal("internal"))
		})
	})
})

func newClientContext(environment string, objects ...crclient.Object) context.Context {
	sch := runtime.NewScheme()
	Expect(apiv1.AddToScheme(sch)).To(Succeed())
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(objects...).Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}
