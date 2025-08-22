// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

func NewApiCategory(name string) *apiv1.ApiCategory {
	return &apiv1.ApiCategory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: apiv1.ApiCategorySpec{
			TagValue:    labelutil.NormalizeValue(name),
			Active:      true,
			Description: "Test category for API controller",
			AllowTeams: &apiv1.AllowTeamsConfig{
				Categories: []string{"test-category"},
				Names:      []string{"test-team"},
			},
			MustHaveGroupPrefix: true,
			Linting: &apiv1.LintingConfig{
				Enabled: false,
				Ruleset: "owasp",
			},
		},
	}
}

var _ = Describe("ApiCategory Controller", func() {

	Context("Creating", func() {

		apiCategory := NewApiCategory("test-category")

		It("should successfully provision the API category resource", func() {
			By("Creating the API category resource")
			err := k8sClient.Create(ctx, apiCategory)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				fetchedCategory := &apiv1.ApiCategory{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: apiCategory.Name, Namespace: testNamespace}, fetchedCategory)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(fetchedCategory.Spec.TagValue).To(Equal(labelutil.NormalizeValue(apiCategory.Name)))
				g.Expect(fetchedCategory.Spec.Active).To(BeTrue())

				g.Expect(fetchedCategory.Status.Conditions).To(HaveLen(2))

			}, timeout, interval).Should(Succeed())
		})
	})
})
