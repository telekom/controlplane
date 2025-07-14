// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var _ = Describe("Application Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		var application *applicationv1.Application
		var zoneA *adminv1.Zone
		var zoneB *adminv1.Zone

		BeforeEach(func() {
			By("Checking if the Namespace is created")
			zoneNamespace := corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "default"}, &zoneNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("creating the Zone A")
			zoneA = &adminv1.Zone{
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
			Expect(k8sClient.Create(ctx, zoneA)).To(Succeed())

			By("creating the Zone B")
			zoneB = &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-b",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: adminv1.ZoneSpec{
					Visibility: adminv1.ZoneVisibilityWorld,
				},
			}
			Expect(k8sClient.Create(ctx, zoneB)).To(Succeed())

		})

		AfterEach(func() {
			By("Cleanup Zone A")
			Expect(k8sClient.Delete(ctx, zoneA)).To(Succeed())

			By("Cleanup Zone B")
			Expect(k8sClient.Delete(ctx, zoneB)).To(Succeed())
		})

		It("should successfully create an application", func() {
			By("Creating the Application resource")
			application = &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-application-1",
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
					Zone:          *ctypes.ObjectRefFromObject(zoneA),
				},
			}

			Expect(k8sClient.Create(ctx, application)).To(Succeed())

			Eventually(func(g Gomega) {
				By("Checking if the Application is created and all conditions are set")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-application-1",
					Namespace: testNamespace,
				}, application)

				expectedClientId := "test-my-team--test-application-1"

				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(application.Status.ClientId).To(Equal(expectedClientId))
				g.Expect(application.Status.ClientSecret).To(Equal("c6283fd0-77f2-452c-8437-4882cffde8e1"))

				g.Expect(application.Status.Clients).To(HaveLen(1))
				g.Expect(application.Status.Consumers).To(HaveLen(1))

				expectedResourceName := expectedClientId + "--zone-a"

				By("Checking if the Identity-Client is created")
				CheckStatusOfClient(ctx, g, expectedClientId, expectedResourceName, testNamespace)

				By("Checking if the Gateway-Consumer is created")
				CheckStatusOfConsumer(ctx, g, expectedClientId, expectedResourceName, testNamespace)

			}, timeout, interval).Should(Succeed())
		})

		It("should successfully create an application with failover configured", func() {
			By("Creating the Application resource")
			application = &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-application-2",
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
					Zone:          *ctypes.ObjectRefFromObject(zoneA),
					FailoverZones: []ctypes.ObjectRef{
						*ctypes.ObjectRefFromObject(zoneB),
					},
				},
			}

			Expect(k8sClient.Create(ctx, application)).To(Succeed())

			Eventually(func(g Gomega) {
				By("Checking if the Application is created and all conditions are set")
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-application-2",
					Namespace: testNamespace,
				}, application)

				g.Expect(err).NotTo(HaveOccurred())

				expectedClientId := "test-my-team--test-application-2"

				g.Expect(application.Status.ClientId).To(Equal(expectedClientId))
				g.Expect(application.Status.ClientSecret).To(Equal("c6283fd0-77f2-452c-8437-4882cffde8e1"))

				g.Expect(application.Status.Clients).To(HaveLen(2))
				g.Expect(application.Status.Consumers).To(HaveLen(2))

				expectedResourceName := expectedClientId + "--zone-a"

				By("Checking if the Identity-Client is created")
				CheckStatusOfClient(ctx, g, expectedClientId, expectedResourceName, testNamespace)

				By("Checking if the Gateway-Consumer is created")
				CheckStatusOfConsumer(ctx, g, expectedClientId, expectedResourceName, testNamespace)

				expectedResourceName = expectedClientId + "--zone-b"

				By("Checking if the failover Identity-Client is created")
				CheckStatusOfClient(ctx, g, expectedClientId, expectedResourceName, testNamespace)

				By("Checking if the failover Gateway-Consumer is created")
				CheckStatusOfConsumer(ctx, g, expectedClientId, expectedResourceName, testNamespace)

			}, timeout, interval).Should(Succeed())
		})

	})
})

func CheckStatusOfClient(ctx context.Context, g Gomega, clientId, name, namespace string) {
	idpClient := &identityv1.Client{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}, idpClient)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(idpClient.Spec.ClientId).To(Equal(clientId))
}

func CheckStatusOfConsumer(ctx context.Context, g Gomega, clientId, name string, namespace string) {
	consumer := &gatewayv1.Consumer{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}, consumer)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(consumer.Spec.Name).To(Equal(clientId))
}
