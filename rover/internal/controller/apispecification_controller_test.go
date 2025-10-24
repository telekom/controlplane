// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("ApiSpecification Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-api"

		ctx := context.Background()

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		obj := &roverv1.ApiSpecification{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ApiSpecification")
			err := k8sClient.Get(ctx, typeNamespacedName, obj)
			if err != nil && errors.IsNotFound(err) {
				obj = &roverv1.ApiSpecification{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: roverv1.ApiSpecificationSpec{
						Specification: "some-random-id",
						Category:      "other",
						BasePath:      "/eni/api/v1",
						Hash:          "someHash",
						Oauth2Scopes:  []string{"read", "write"},
						XVendor:       true,
						Version:       "1.0.0",
					},
				}
				Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &roverv1.ApiSpecification{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ApiSpecification")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: testNamespace,
				}, obj)

				g.Expect(err).NotTo(HaveOccurred())

				api := &apiapi.Api{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "eni-api-v1",
					Namespace: testNamespace,
				}, api)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(api.Spec.Version).To(Equal("1.0.0"))
				g.Expect(api.Spec.XVendor).To(Equal(true))
				g.Expect(api.Spec.Category).To(Equal("other"))
				g.Expect(api.Spec.Oauth2Scopes).To(ConsistOf("read", "write"))

			}, timeout, interval).Should(Succeed())
		})
	})
})
