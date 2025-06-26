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
		const openapiSpec = `
openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
  x-api-category: Test
  x-vendor: true
servers:
  - url: http://localhost:8080/eni/api/v1y
components:
  securitySchemes:
    oAuth2:
      type: oauth2
      description: dummy oauth2
      flows:
        clientCredentials:
          tokenUrl: >-
            http://localhost:8080/proxy/auth/realms/default/protocol/openid-connect/token
          scopes:
            read: read dummy
            write: write dummy
`

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
						Specification: openapiSpec,
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
				g.Expect(api.Spec.Name).To(Equal("eni-api-v1"))
				g.Expect(api.Spec.Version).To(Equal("1.0.0"))
				g.Expect(api.Spec.XVendor).To(Equal(true))
				g.Expect(api.Spec.Category).To(Equal("other"))
				g.Expect(api.Spec.Security.Authentication.OAuth2.Scopes).To(ConsistOf("read", "write"))

			}, timeout, interval).Should(Succeed())
		})
	})
})
