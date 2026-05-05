// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("ApiChangelog Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-apichangelog"

		ctx := context.Background()

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		obj := &roverv1.ApiChangelog{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ApiChangelog")
			err := k8sClient.Get(ctx, typeNamespacedName, obj)
			if err != nil && errors.IsNotFound(err) {
				obj = &roverv1.ApiChangelog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: roverv1.ApiChangelogSpec{
						SpecificationRef: types.TypedObjectRef{
							TypeMeta: metav1.TypeMeta{
								Kind:       "ApiSpecification",
								APIVersion: "rover.cp.ei.telekom.de/v1",
							},
							ObjectRef: types.ObjectRef{
								Name:      "eni-my-api",
								Namespace: testNamespace,
							},
						},
						Contents: "test--eni--team--my-api-v1",
						Hash:     "abc123hash",
					},
				}
				Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &roverv1.ApiChangelog{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ApiChangelog")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, obj)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify conditions are set correctly
				g.Expect(obj.Status.Conditions).NotTo(BeEmpty())

				// Check for Ready condition
				readyCondition := false
				processingCondition := false
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						readyCondition = true
					}
					if cond.Type == "Processing" && cond.Status == metav1.ConditionFalse {
						processingCondition = true
					}
				}

				g.Expect(readyCondition).To(BeTrue(), "Ready condition should be true")
				g.Expect(processingCondition).To(BeTrue(), "Processing condition should be false")

			}, timeout, interval).Should(Succeed())
		})
	})
})
