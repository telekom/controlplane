// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	config "github.com/telekom/controlplane/common/pkg/config"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SFTPServiceConfig Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		const resourceName = "test-sftpserviceconfig"

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		testSFTPServiceConfig := &sftpv1.SFTPServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: "test",
				},
			},
			Spec: sftpv1.SFTPServiceConfigSpec{
				API: sftpv1.APIEndpoint{
					ClientID:     "client-id",
					ClientSecret: "secret-manager://path/to/secret",
					Endpoint:     "https://issuer.example.de/oauth/token",
					Issuer:       "https://issuer.example.de",
				},
			},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SFTPServiceConfig")
			resource := &sftpv1.SFTPServiceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testSFTPServiceConfig)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &sftpv1.SFTPServiceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SFTPServiceConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifySFTPServiceConfig(ctx, g, typeNamespacedName)
			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifySFTPServiceConfig(ctx context.Context, g Gomega, namespacedName client.ObjectKey) {
	By("Checking if the SFTPServiceConfig is created and all conditions are set")
	sftpServiceConfig := &sftpv1.SFTPServiceConfig{}
	err := k8sClient.Get(ctx, namespacedName, sftpServiceConfig)
	g.Expect(err).NotTo(HaveOccurred())

	ready := meta.FindStatusCondition(sftpServiceConfig.Status.Conditions, condition.ConditionTypeReady)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.ObservedGeneration).To(Equal(sftpServiceConfig.Generation))
	g.Expect(sftpServiceConfig.Status.Conditions).To(HaveLen(2))
	g.Expect(meta.IsStatusConditionTrue(sftpServiceConfig.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(sftpServiceConfig.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
