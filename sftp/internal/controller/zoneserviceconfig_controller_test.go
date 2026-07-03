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

var _ = Describe("ZoneServiceConfig Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		const resourceName = "test-zoneserviceconfig"

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		testZSC := &sftpv1.ZoneServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: "test",
				},
			},
			Spec: sftpv1.ZoneServiceConfigSpec{
				API: sftpv1.APIEndpoint{
					ClientID:     "client-id",
					ClientSecret: "secret-manager://path/to/secret",
					Endpoint:     "https://issuer.example.de/oauth/token",
					Issuer:       "https://issuer.example.de",
				},
			},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ZoneServiceConfig")
			resource := &sftpv1.ZoneServiceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testZSC)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &sftpv1.ZoneServiceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ZoneServiceConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyZoneServiceConfig(ctx, g, typeNamespacedName)
			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyZoneServiceConfig(ctx context.Context, g Gomega, namespacedName client.ObjectKey) {
	By("Checking if the ZoneServiceConfig is created and all conditions are set")
	zsc := &sftpv1.ZoneServiceConfig{}
	err := k8sClient.Get(ctx, namespacedName, zsc)
	g.Expect(err).NotTo(HaveOccurred())

	ready := meta.FindStatusCondition(zsc.Status.Conditions, condition.ConditionTypeReady)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.ObservedGeneration).To(Equal(zsc.Generation))
	g.Expect(zsc.Status.Conditions).To(HaveLen(2))
	g.Expect(meta.IsStatusConditionTrue(zsc.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(zsc.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
