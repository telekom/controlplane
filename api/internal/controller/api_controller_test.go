// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0
package controller

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
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
				apiv1.BasePathLabelKey:     labelutil.NormalizeValue(apiBasePath),
			},
		},
		Spec: apiv1.ApiSpec{
			Name:     labelutil.NormalizeValue(apiBasePath),
			Version:  "v1",
			BasePath: apiBasePath,
			Category: "other",
			Security: &apiv1.Security{
				Authentication: &apiv1.Authentication{
					OAuth2: &apiv1.OAuth2{
						Scopes: []string{"scope1", "scope2"},
					},
				},
			},
			XVendor: false,
		},
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

			}, timeout, interval).Should(Succeed())
		})

	})
})
