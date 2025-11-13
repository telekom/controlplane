// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0
package controller

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func NewApi(apiBasePath string) *apiv1.Api {
	return &apiv1.Api{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiBasePath),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(apiBasePath),
			},
		},
		Spec: apiv1.ApiSpec{
			Version:      "v1",
			BasePath:     apiBasePath,
			Category:     "other",
			Oauth2Scopes: []string{"scope1", "scope2", "team:scope", "api:scope"},
			XVendor:      false,
		},
		Status: apiv1.ApiStatus{},
	}
}

var _ = Describe("Api Controller", func() {

	Context("Creating, Updating and ActiveSwitch", Ordered, func() {

		var apiBasePath = "/apictrl/test/v1"
		var firstApi *apiv1.Api
		var secondApi *apiv1.Api

		BeforeAll(func() {
			firstApi = NewApi(apiBasePath)
			secondApi = NewApi(apiBasePath)
			secondApi.Name = "another-test-api"
		})

		AfterAll(func() {
			By("Cleaning up and deleting all resources")
			err := k8sClient.Delete(ctx, secondApi)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should successfully provision the API resource and set it to active", func() {
			By("Creating the API resource")
			err := k8sClient.Create(ctx, firstApi)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(firstApi), firstApi)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(firstApi.Status.Active).To(BeTrue())
				testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(firstApi.GetConditions(), condition.ConditionTypeReady), "ApiActive")

			}, timeout, interval).Should(Succeed())
		})

		It("should successfully provision the API resource and set it to inactive", func() {
			By("Creating the second API resource")
			err := k8sClient.Create(ctx, secondApi)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApi), secondApi)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApi.Status.Active).To(BeFalse())
				testutil.ExpectConditionToBeFalse(g, meta.FindStatusCondition(secondApi.GetConditions(), condition.ConditionTypeReady), "ApiNotActive")

			}, timeout, interval).Should(Succeed())
		})

		It("should switch the API resource from inactive to active", func() {
			By("Deleting the first API resource")
			err := k8sClient.Delete(ctx, firstApi)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the second API resource is active")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApi), secondApi)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApi.Status.Active).To(BeTrue())
				testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(secondApi.GetConditions(), condition.ConditionTypeReady), "ApiActive")

			}, timeout, interval).Should(Succeed())
		})

	})

	Context("Validating the basePath", Ordered, func() {

		var apiBasePath = "/apictrl/test1/v1"
		var firstApi *apiv1.Api
		var secondApi *apiv1.Api

		BeforeAll(func() {
			firstApi = NewApi(apiBasePath)
			secondApi = NewApi(apiBasePath)
			secondApi.Name = "another-test-api"
			secondApi.Spec.BasePath = "/APICtrl/Test1/v1" // different case
		})

		AfterAll(func() {
			By("Cleaning up and deleting all resources")
			err := k8sClient.Delete(ctx, firstApi)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, secondApi)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should successfully provision the first API resource", func() {
			By("Creating the first API resource")
			err := k8sClient.Create(ctx, firstApi)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(firstApi), firstApi)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(firstApi.Status.Active).To(BeTrue())
				testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(firstApi.GetConditions(), condition.ConditionTypeReady), "ApiActive")

			}, timeout, interval).Should(Succeed())
		})

		It("should block the second API resource due to basePath conflict", func() {
			By("Creating the second API resource")
			err := k8sClient.Create(ctx, secondApi)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApi), secondApi)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApi.Status.Active).To(BeFalse())
				testutil.ExpectConditionToBeFalse(g, meta.FindStatusCondition(secondApi.GetConditions(), condition.ConditionTypeReady), "ApiNotActiveCaseConflict")

			}, timeout, interval).Should(Succeed())
		})

	})
})
