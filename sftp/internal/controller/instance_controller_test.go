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
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Instance Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()
		const instanceName = "test-instance"
		const sftpServiceConfigName = "test-sftpserviceconfig-for-instance"

		instanceKey := client.ObjectKey{Name: instanceName, Namespace: testNamespace}
		sftpServiceConfigKey := client.ObjectKey{Name: sftpServiceConfigName, Namespace: testNamespace}

		testSFTPServiceConfig := &sftpv1.SFTPServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: sftpServiceConfigName, Namespace: testNamespace,
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: sftpv1.SFTPServiceConfigSpec{
				API: sftpv1.APIEndpoint{
					ClientID:     "client-id",
					ClientSecret: "secret-manager://path/to/secret",
					Endpoint:     "https://example.de/base-path/",
					Issuer:       "https://issuer.example.de/auth/realms/default/protocol/openid-connect/token",
				},
			},
		}

		testInstance := &sftpv1.Instance{
			ObjectMeta: metav1.ObjectMeta{
				Name: instanceName, Namespace: testNamespace,
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: sftpv1.InstanceSpec{
				Description: "Test instance for controller test",
				SFTPServiceConfigRef: commontypes.ObjectRef{
					Name:      sftpServiceConfigName,
					Namespace: testNamespace,
				},
			},
		}

		BeforeEach(func() {
			By("creating required SFTPServiceConfig")
			resource := &sftpv1.SFTPServiceConfig{}
			err := k8sClient.Get(ctx, sftpServiceConfigKey, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testSFTPServiceConfig)).To(Succeed())
			}

			By("creating the custom resource for the Kind Instance")
			instance := &sftpv1.Instance{}
			err = k8sClient.Get(ctx, instanceKey, instance)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testInstance)).To(Succeed())
			}
		})

		AfterEach(func() {
			instance := &sftpv1.Instance{}
			err := k8sClient.Get(ctx, instanceKey, instance)
			Expect(err).NotTo(HaveOccurred())
			By("cleaning up the Instance resource")
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())

			sftpServiceConfig := &sftpv1.SFTPServiceConfig{}
			err = k8sClient.Get(ctx, sftpServiceConfigKey, sftpServiceConfig)
			Expect(err).NotTo(HaveOccurred())
			By("cleaning up the SFTPServiceConfig resource")
			Expect(k8sClient.Delete(ctx, sftpServiceConfig)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyInstance(ctx, g, instanceKey)
			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyInstance(ctx context.Context, g Gomega, namespacedName client.ObjectKey) {
	By("checking if the Instance is created and all conditions are set")
	instance := &sftpv1.Instance{}
	err := k8sClient.Get(ctx, namespacedName, instance)
	g.Expect(err).NotTo(HaveOccurred())

	ready := meta.FindStatusCondition(instance.Status.Conditions, condition.ConditionTypeReady)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.ObservedGeneration).To(Equal(instance.Generation))
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, sftpv1.ConditionTypePublicKeysUpdatedInService)).To(BeTrue())
}
