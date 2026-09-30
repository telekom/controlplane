// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConsumeRoute admission", func() {
	It("accepts consumer username/password together with scopes", func() {
		createNamespace(testEnvironment)
		consumeRoute := &gatewayv1.ConsumeRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "password-consumer",
				Namespace: testEnvironment,
				Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.ConsumeRouteSpec{
				Route:        types.ObjectRef{Name: "password-route", Namespace: testEnvironment},
				ConsumerName: "password-consumer",
				Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Basic:  &gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
					Scopes: []string{"consumer:read", "consumer:write"},
				}},
			},
		}

		Expect(k8sClient.Create(ctx, consumeRoute)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, consumeRoute)).To(Succeed()) })
	})
})

var _ = Describe("External IDP password route reconciliation", func() {
	It("keeps a credential-only route ready when mesh access is added", func() {
		createNamespace(testEnvironment)
		gateway := newGateway("password-mesh-gateway", testEnvironment)
		Expect(k8sClient.Create(ctx, gateway)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, gateway)).To(Succeed()) })
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(gateway.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		route := &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name: "password-mesh-route", Namespace: testEnvironment,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: *types.ObjectRefFromObject(gateway),
				Type:       gatewayv1.RouteTypePrimary,
				Hostnames:  []string{"password.example.com"},
				Paths:      []string{"/password/v1"},
				Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{
					{Scheme: "https", Hostname: "backend.example.com", Port: 443},
				}},
				Security: gatewayv1.Security{
					TrustedIssuers: []string{"https://issuer.example.com/realms/test"},
					M2M: &gatewayv1.Machine2MachineAuthentication{
						ExternalIDP: &gatewayv1.ExternalIdentityProvider{
							TokenEndpoint: "https://idp.example.com/token",
							TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
							GrantType:     gatewayv1.GrantTypePassword,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, route)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, route)).To(Succeed()) })
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		consumer := &gatewayv1.ConsumeRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name: "password-mesh-consumer", Namespace: testEnvironment,
				Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.ConsumeRouteSpec{
				Route:        *types.ObjectRefFromObject(route),
				ConsumerName: "password-subscriber",
				Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Basic:  &gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
					Scopes: []string{"consumer:read"},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, consumer)).To(Succeed()) })
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consumer), consumer)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(consumer.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		By("authorizing the mesh transport without assigning it a backend identity")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			route.Spec.Security.DefaultConsumers = []string{"gateway"}
			g.Expect(k8sClient.Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			ready := meta.FindStatusCondition(route.GetConditions(), condition.ConditionTypeReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.ObservedGeneration).To(Equal(route.Generation))
			g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(route.Status.Consumers).To(ConsistOf("password-subscriber"))
		}, timeout, interval).Should(Succeed())

		By("still rejecting an application default that has no backend credentials")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			route.Spec.Security.DefaultConsumers = []string{gatewayv1.GatewayConsumerName, "uncredentialed-default"}
			g.Expect(k8sClient.Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			ready := meta.FindStatusCondition(route.GetConditions(), condition.ConditionTypeReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.ObservedGeneration).To(Equal(route.Generation))
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal(condition.ReasonValidationFailed))
			g.Expect(route.Status.Consumers).To(BeEmpty())
		}, timeout, interval).Should(Succeed())

		By("recovering after only the mesh transport remains")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			route.Spec.Security.DefaultConsumers = []string{gatewayv1.GatewayConsumerName}
			g.Expect(k8sClient.Update(ctx, route)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			ready := meta.FindStatusCondition(route.GetConditions(), condition.ConditionTypeReady)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.ObservedGeneration).To(Equal(route.Generation))
			g.Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(route.Status.Consumers).To(ConsistOf("password-subscriber"))
		}, timeout, interval).Should(Succeed())
	})
})
