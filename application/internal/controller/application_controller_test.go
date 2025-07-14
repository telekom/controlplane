// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	cptypes "github.com/telekom/controlplane/common/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var _ = Describe("Application Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-application"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		application := &applicationv1.Application{}
		zone := &adminv1.Zone{}
		gateway := &gatewayv1.Gateway{}

		BeforeEach(func() {
			By("Checking if the Namespace is created")
			zoneNamespace := corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, &zoneNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("creating the custom resource for the Kind Zone")
			err = k8sClient.Get(ctx, typeNamespacedName, zone)
			if err != nil && errors.IsNotFound(err) {
				resource := &adminv1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "zone-a",
						Namespace: testNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: adminv1.ZoneSpec{
						Visibility: adminv1.ZoneVisibilityWorld,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Gateway")
			err = k8sClient.Get(ctx, typeNamespacedName, gateway)
			if err != nil && errors.IsNotFound(err) {
				resource := &gatewayv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "stargate",
						Namespace: testNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: gatewayv1.GatewaySpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Application")
			err = k8sClient.Get(ctx, typeNamespacedName, application)
			if err != nil && errors.IsNotFound(err) {
				zoneRef := cptypes.ObjectRef{
					Name:      "zone-a",
					Namespace: testNamespace,
				}
				resource := &applicationv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: testNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: applicationv1.ApplicationSpec{
						Team:          "test-my-team",
						TeamEmail:     "test-DTIT_ENI_Hub_Team_Hyperion@telekom.de",
						Secret:        "c6283fd0-77f2-452c-8437-4882cffde8e1",
						NeedsClient:   true,
						NeedsConsumer: true,
						Zone:          zoneRef,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &applicationv1.Application{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Application")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())

			gatewayResource := &gatewayv1.Gateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: "stargate", Namespace: testNamespace,
			}, gatewayResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Gateway")
			Expect(k8sClient.Delete(ctx, gatewayResource)).To(Succeed())

			zoneResource := &adminv1.Zone{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "zone-a", Namespace: testNamespace}, zoneResource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Zone")
			Expect(k8sClient.Delete(ctx, zoneResource)).To(Succeed())

		})
		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				By("Checking if the Application is created and all conditions are set")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      resourceName,
					Namespace: testNamespace,
				}, application)

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(application.Status.ClientId).To(Equal("test-my-team--test-application"))
				g.Expect(application.Status.ClientSecret).To(Equal("c6283fd0-77f2-452c-8437-4882cffde8e1"))
				g.Expect(meta.IsStatusConditionTrue(application.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
				g.Expect(meta.IsStatusConditionTrue(application.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				By("Checking if the Identity-Client is created")
				idpClient := &identityv1.Client{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-my-team--test-application",
					Namespace: testNamespace,
				}, idpClient)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(idpClient.Spec.ClientId).To(Equal("test-my-team--test-application"))
				g.Expect(idpClient.Spec.ClientSecret).To(Equal("c6283fd0-77f2-452c-8437-4882cffde8e1"))

				By("Checking if the Gateway-Consumer is created")
				consumer := &gatewayv1.Consumer{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-my-team--test-application",
					Namespace: testNamespace,
				}, consumer)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(consumer.Spec.Name).To(Equal("test-my-team--test-application"))
				g.Expect(consumer.Spec.Realm.Name).To(Equal("test-env"))

			}, timeout, interval).Should(Succeed())
		})
	})
})
