// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRealm(name string) *gatewayv1.Realm {
	return &gatewayv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey:   testEnvironment,
				config.BuildLabelKey("zone"): "test",
			},
		},
		Spec: gatewayv1.RealmSpec{
			Urls:       []string{"https://realm.url"},
			IssuerUrls: []string{"https://issuer.url"},
			DefaultConsumers: []string{
				"test",
			},
		},
	}
}

var _ = Describe("Realm Controller", Ordered, func() {

	var gateway *gatewayv1.Gateway
	var realm *gatewayv1.Realm

	BeforeAll(func() {
		By("Initializing the Gateway and Realm")
		gateway = NewGateway("test-realm")
		realm = NewRealm("test-realm")

		By("Creating the gateway")
		err := k8sClient.Create(ctx, gateway)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Tearing down the Gateway")
		err := k8sClient.Delete(ctx, gateway)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Virtual Realm", func() {
		It("should be ready ", func() {
			err := k8sClient.Create(ctx, realm)
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the Realm is ready")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(realm), realm)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking the conditions")
				g.Expect(realm.Status.Conditions).To(HaveLen(2))
				readyCondition := meta.FindStatusCondition(realm.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Real Realm", func() {

		It("should create the realm routes", func() {
			realm.Spec.Gateway = &types.ObjectRef{
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			}
			err := k8sClient.Update(ctx, realm)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(realm), realm)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking the conditions")
				g.Expect(realm.Status.Conditions).To(HaveLen(2))
				readyCondition := meta.FindStatusCondition(realm.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).NotTo(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

				By("Checking the routes")
				g.Expect(realm.Status.CertsRoute).NotTo(BeNil())
				g.Expect(realm.Status.CertsUrl).To(Equal("https://realm.url:443/auth/realms/test-realm/protocol/openid-connect/certs"))
				g.Expect(realm.Status.DiscoveryRoute).NotTo(BeNil())
				g.Expect(realm.Status.DiscoveryUrl).To(Equal("https://realm.url:443/auth/realms/test-realm/.well-known/openid-configuration"))
				g.Expect(realm.Status.IssuerRoute).NotTo(BeNil())
				g.Expect(realm.Status.IssuerUrl).To(Equal("https://realm.url:443/auth/realms/test-realm"))

			}, timeout, interval).Should(Succeed())

			err = k8sClient.Delete(ctx, realm)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(realm), realm)
				g.Expect(err).To(HaveOccurred())

			}, timeout, interval).Should(Succeed())

		})
	})

	Context("URL Validation", func() {
		It("should reject invalid URLs", func() {
			invalidRealm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-realm",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey:   testEnvironment,
						config.BuildLabelKey("zone"): "test",
					},
				},
				Spec: gatewayv1.RealmSpec{
					Urls:             []string{"not-a-valid-url"},
					IssuerUrls:       []string{"also-not-valid"},
					DefaultConsumers: []string{},
				},
			}

			By("Attempting to create a realm with invalid URLs")
			err := k8sClient.Create(ctx, invalidRealm)
			Expect(err).To(HaveOccurred(), "Expected validation error for invalid URLs")
		})

		It("should accept valid URLs", func() {
			validRealm := &gatewayv1.Realm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-realm",
					Namespace: testNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey:   testEnvironment,
						config.BuildLabelKey("zone"): "test",
					},
				},
				Spec: gatewayv1.RealmSpec{
					Urls:             []string{"https://valid.example.com", "http://another.example.com:8080/path"},
					IssuerUrls:       []string{"https://issuer.example.com/auth/realms/test"},
					DefaultConsumers: []string{},
				},
			}

			By("Creating a realm with valid URLs")
			err := k8sClient.Create(ctx, validRealm)
			Expect(err).NotTo(HaveOccurred(), "Valid URLs should be accepted")

			By("Cleaning up the valid realm")
			err = k8sClient.Delete(ctx, validRealm)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
