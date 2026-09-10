// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

	Context("When mapping SFTPServiceConfig changes", func() {
		It("uses the SFTPServiceConfig reference field index to find Instances", func() {
			scheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
			Expect(sftpv1.AddToScheme(scheme)).To(Succeed())

			const (
				sftpServiceConfigName = "indexed-sftpserviceconfig"
				matchingInstanceName  = "matching-instance"
				otherInstanceName     = "other-instance"
			)

			sftpServiceConfig := &sftpv1.SFTPServiceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sftpServiceConfigName,
					Namespace: testNamespace,
				},
			}
			matchingInstance := &sftpv1.Instance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      matchingInstanceName,
					Namespace: testNamespace,
				},
				Spec: sftpv1.InstanceSpec{
					SFTPServiceConfigRef: commontypes.ObjectRef{
						Name:      sftpServiceConfigName,
						Namespace: testNamespace,
					},
				},
			}
			otherInstance := &sftpv1.Instance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      otherInstanceName,
					Namespace: testNamespace,
				},
				Spec: sftpv1.InstanceSpec{
					SFTPServiceConfigRef: commontypes.ObjectRef{
						Name:      "other-sftpserviceconfig",
						Namespace: testNamespace,
					},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(matchingInstance, otherInstance).
				WithIndex(&sftpv1.Instance{}, sftpv1.IndexFieldSpecSFTPServiceConfigRef, func(obj client.Object) []string {
					instance, ok := obj.(*sftpv1.Instance)
					if !ok || instance.Spec.SFTPServiceConfigRef.IsEmpty() {
						return nil
					}
					return []string{instance.Spec.SFTPServiceConfigRef.String()}
				}).
				Build()
			reconciler := &InstanceReconciler{Client: k8sClient}

			reqs := reconciler.MapSFTPServiceConfigToInstance(context.Background(), sftpServiceConfig)

			Expect(reqs).To(HaveLen(1))
			Expect(reqs[0].NamespacedName).To(Equal(client.ObjectKeyFromObject(matchingInstance)))
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
}
