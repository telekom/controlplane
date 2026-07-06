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

var _ = Describe("User Controller", func() {
	Context("When reconciling a User", func() {
		ctx := context.Background()
		const (
			instanceName          = "test-instance-for-user"
			sftpServiceConfigName = "test-sftpserviceconfig-for-user"
			userName              = "test-user"
		)

		instanceKey := client.ObjectKey{Name: instanceName, Namespace: testNamespace}
		sftpServiceConfigKey := client.ObjectKey{Name: sftpServiceConfigName, Namespace: testNamespace}
		userKey := client.ObjectKey{Name: userName, Namespace: testNamespace}

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
				Description: "Test instance for user controller test",
				SFTPServiceConfigRef: commontypes.ObjectRef{
					Name:      sftpServiceConfigName,
					Namespace: testNamespace,
				},
			},
		}

		testUser := &sftpv1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: userName, Namespace: testNamespace,
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: sftpv1.UserSpec{
				InstanceRef: commontypes.ObjectRef{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				SSHPublicKeys: []string{"ssh-rsa cHJvdmlkZXI= provider@example.com"},
			},
		}

		BeforeEach(func() {
			By("creating required SFTPServiceConfig")
			resource := &sftpv1.SFTPServiceConfig{}
			err := k8sClient.Get(ctx, sftpServiceConfigKey, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testSFTPServiceConfig.DeepCopy())).To(Succeed())
			}

			By("creating the Instance resource")
			instance := &sftpv1.Instance{}
			err = k8sClient.Get(ctx, instanceKey, instance)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testInstance.DeepCopy())).To(Succeed())
			}

			By("creating the User resource")
			user := &sftpv1.User{}
			err = k8sClient.Get(ctx, userKey, user)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testUser.DeepCopy())).To(Succeed())
			}
		})

		AfterEach(func() {
			user := &sftpv1.User{}
			err := k8sClient.Get(ctx, userKey, user)
			if err == nil {
				By("cleaning up the User resource")
				Expect(k8sClient.Delete(ctx, user)).To(Succeed())
			}

			instance := &sftpv1.Instance{}
			err = k8sClient.Get(ctx, instanceKey, instance)
			if err == nil {
				By("cleaning up the Instance resource")
				Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
			}

			sftpServiceConfig := &sftpv1.SFTPServiceConfig{}
			err = k8sClient.Get(ctx, sftpServiceConfigKey, sftpServiceConfig)
			if err == nil {
				By("cleaning up the SFTPServiceConfig resource")
				Expect(k8sClient.Delete(ctx, sftpServiceConfig)).To(Succeed())
			}
		})

		It("projects Instance user processing status onto User status", func() {
			Eventually(func(g Gomega) {
				VerifyUser(ctx, g, userKey)
			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyUser(ctx context.Context, g Gomega, namespacedName client.ObjectKey) {
	By("checking if the User status is projected from the Instance")
	user := &sftpv1.User{}
	err := k8sClient.Get(ctx, namespacedName, user)
	g.Expect(err).NotTo(HaveOccurred())

	ready := meta.FindStatusCondition(user.Status.Conditions, condition.ConditionTypeReady)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.ObservedGeneration).To(Equal(user.Generation))
	g.Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(user.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
