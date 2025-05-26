// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
)

var _ = Describe("Environment Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		const resourceName = "test-resource"

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		testEnv := &adminv1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: adminv1.EnvironmentSpec{},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Environment")
			resource := &adminv1.Environment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testEnv)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &adminv1.Environment{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Environment")
			Expect(k8sClient.Delete(ctx, testEnv)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyEnvironment(ctx, g, typeNamespacedName, testEnv)
			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyEnvironment(ctx context.Context, g Gomega, namespacedName client.ObjectKey, envToVerify *adminv1.Environment) {
	By("Checking if the Environment is created and all conditions are set")
	env := &adminv1.Environment{}
	err := k8sClient.Get(ctx, namespacedName, env)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(env.Spec).To(Equal(envToVerify.Spec))

	g.Expect(env.Status.Conditions).To(HaveLen(2))
	g.Expect(meta.IsStatusConditionTrue(env.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(env.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
