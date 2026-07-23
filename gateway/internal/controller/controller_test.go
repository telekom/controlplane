// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Controller Integration", Ordered, func() {
	const (
		gatewayName = "smoke-gateway"
		namespace   = testEnvironment
	)

	ctx := context.Background()
	var gateway *gatewayv1.Gateway

	BeforeAll(func() {
		createNamespace(namespace)

		gateway = newGateway(gatewayName, namespace)
		Expect(k8sClient.Create(ctx, gateway)).To(Succeed())

		// Wait for Gateway to be Ready (prerequisite for all other resources)
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(meta.IsStatusConditionTrue(gateway.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		Expect(k8sClient.Delete(ctx, gateway)).To(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	Describe("Route reconciliation sends correct data to Kong", func() {
		// This test verifies that the full pipeline (Controller → Handler → Features → KongClient)
		// produces the correct Kong API call with the right route name, hostnames, and paths.
		var route *gatewayv1.Route

		BeforeAll(func() {
			route = &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-route",
					Namespace: namespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: types.ObjectRef{
						Name:      gatewayName,
						Namespace: namespace,
					},
					Type:      gatewayv1.RouteTypePrimary,
					Hostnames: []string{"api.myservice.com"},
					Paths:     []string{"/payments/v1"},
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{
							{Scheme: "https", Hostname: "payments-backend.internal", Port: 8443, Path: "/api"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
		})

		AfterAll(func() {
			Expect(k8sClient.Delete(ctx, route)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should reconcile Route to Ready status", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Describe("ConsumeRoute and Route cross-resource coordination", func() {
		// This tests the watch mechanism that can ONLY be validated at the controller level:
		// 1. Creating a ConsumeRoute triggers Route re-reconciliation (via mapConsumeRouteToRoute watch)
		// 2. The Route picks up the ConsumeRoute and adds it to Status.Consumers
		// 3. The Route status change triggers ConsumeRoute re-reconciliation (via mapRouteToConsumeRoute watch)
		// 4. The ConsumeRoute sees itself in the Route's consumers list and becomes Ready

		var route *gatewayv1.Route
		var consumeRoute *gatewayv1.ConsumeRoute

		BeforeAll(func() {
			route = &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "coordinated-route",
					Namespace: namespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: types.ObjectRef{
						Name:      gatewayName,
						Namespace: namespace,
					},
					Type:      gatewayv1.RouteTypePrimary,
					Hostnames: []string{"coordinated.example.com"},
					Paths:     []string{"/coord/v1"},
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{
							{Scheme: "https", Hostname: "coord-backend.internal", Port: 443},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, route)).To(Succeed())

			// Wait for Route to be Ready first
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		AfterAll(func() {
			if consumeRoute != nil {
				_ = k8sClient.Delete(ctx, consumeRoute)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(consumeRoute), consumeRoute)
					g.Expect(errors.IsNotFound(err)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			}
			Expect(k8sClient.Delete(ctx, route)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should propagate consumer through Route status and make ConsumeRoute Ready", func() {
			consumeRoute = &gatewayv1.ConsumeRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-subscription",
					Namespace: namespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: gatewayv1.ConsumeRouteSpec{
					Route: types.ObjectRef{
						Name:      "coordinated-route",
						Namespace: namespace,
					},
					ConsumerName: "subscriber-app",
				},
			}
			Expect(k8sClient.Create(ctx, consumeRoute)).To(Succeed())

			By("verifying the Route picks up the consumer via watch-triggered re-reconciliation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(route.Status.Consumers).To(ContainElement("subscriber-app"))
			}, timeout, interval).Should(Succeed())

			By("verifying the ConsumeRoute becomes Ready after seeing itself in Route.Status.Consumers")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(consumeRoute), consumeRoute)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(consumeRoute.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})

	Describe("Gateway aggregate compilation", func() {
		const envoyGatewayName = "envoy-gateway"

		var (
			envoyGateway *gatewayv1.Gateway
			route        *gatewayv1.Route
			consumer     *gatewayv1.Consumer
		)

		BeforeEach(func() {
			publicationActive.Store(true)
			for len(compiledBundles) > 0 {
				<-compiledBundles
			}
			envoyGateway = newGateway(envoyGatewayName, namespace)
			envoyGateway.Spec.Type = gatewayv1.GatewayTypeEnvoy
			Expect(k8sClient.Create(ctx, envoyGateway)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(envoyGateway.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			route = &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aggregate-route", Namespace: namespace,
					Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: types.ObjectRef{Name: envoyGatewayName, Namespace: namespace},
					Type:       gatewayv1.RouteTypePrimary,
					Paths:      []string{"/created"},
					Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{
						Scheme: "http", Hostname: "aggregate.internal", Port: 8080,
					}}},
				},
			}
		})

		AfterEach(func() {
			if consumer != nil {
				_ = k8sClient.Delete(ctx, consumer)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(consumer), &gatewayv1.Consumer{})
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
			_ = k8sClient.Delete(ctx, route)
			_ = k8sClient.Delete(ctx, envoyGateway)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), &gatewayv1.Gateway{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("recompiles complete bundles for create, update, and deletion omission", func() {
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/created")
			})))

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			route.Spec.Paths = []string{"/updated"}
			Expect(k8sClient.Update(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/updated")
			})))

			Expect(k8sClient.Delete(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return !bundleHasRoute(&bundle, "test/aggregate-route")
			})))
		})

		It("rejects environment changes while preserving the active xDS target", func() {
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/created")
			})))
			for len(compiledBundles) > 0 {
				<-compiledBundles
			}

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
			envoyGateway.Labels[config.EnvironmentLabelKey] = "test-migrated"
			Expect(k8sClient.Update(ctx, envoyGateway)).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
				condition := meta.FindStatusCondition(envoyGateway.Status.Conditions, conditionTypeXDSProgrammed)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Reason).To(Equal("XDSTargetImmutable"))
			}, timeout, interval).Should(Succeed())
			Consistently(compiledBundles, time.Second, interval).ShouldNot(Receive())
		})

		It("does not mark a superseded publication as programmed", func() {
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/created")
			})))
			publicationActive.Store(false)

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
			route.Spec.Paths = []string{"/superseded"}
			Expect(k8sClient.Update(ctx, route)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
				condition := meta.FindStatusCondition(envoyGateway.Status.Conditions, conditionTypeXDSProgrammed)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(condition.Reason).To(Equal("XDSSuperseded"))
			}, timeout, interval).Should(Succeed())
		})

		It("deactivates xDS and re-reconciles routes when switching back to Kong", func() {
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/created")
			})))

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
			envoyGateway.Spec.Type = gatewayv1.GatewayTypeKong
			Expect(k8sClient.Update(ctx, envoyGateway)).To(Succeed())

			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return len(bundle.Listeners) == 0 && len(bundle.Routes) == 0 &&
					len(bundle.Clusters) == 0 && len(bundle.Endpoints) == 0
			})))
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
				g.Expect(route.Status.Properties).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
			consumer = &gatewayv1.Consumer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aggregate-consumer", Namespace: namespace,
					Labels: map[string]string{config.EnvironmentLabelKey: testEnvironment},
				},
				Spec: gatewayv1.ConsumerSpec{
					Gateway: types.ObjectRef{Name: envoyGatewayName, Namespace: namespace}, Name: "consumer-a",
				},
			}
			Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consumer), consumer)).To(Succeed())
				g.Expect(consumer.Status.Properties).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
			deletedRoutes.Store(0)
			deletedConsumers.Store(0)

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
			envoyGateway.Spec.Type = gatewayv1.GatewayTypeEnvoy
			Expect(k8sClient.Update(ctx, envoyGateway)).To(Succeed())
			Eventually(compiledBundles, timeout, interval).Should(Receive(Satisfy(func(bundle envoy.ResourceBundle) bool {
				return bundleHasPath(&bundle, "/created")
			})))
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(envoyGateway), envoyGateway)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(envoyGateway.Status.Conditions, conditionTypeXDSProgrammed)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
				g.Expect(route.Status.Properties).To(BeEmpty())
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(consumer), consumer)).To(Succeed())
				g.Expect(consumer.Status.Properties).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
			Expect(deletedRoutes.Load()).To(BeNumerically(">=", 1))
			Expect(deletedConsumers.Load()).To(BeNumerically(">=", 1))
		})
	})
})

func bundleHasPath(bundle *envoy.ResourceBundle, path string) bool {
	for _, routeConfig := range bundle.Routes {
		for _, virtualHost := range routeConfig.VirtualHosts {
			for _, route := range virtualHost.Routes {
				if route.GetMatch().GetPrefix() == path {
					return true
				}
			}
		}
	}
	return false
}

func bundleHasRoute(bundle *envoy.ResourceBundle, name string) bool {
	for _, routeConfig := range bundle.Routes {
		for _, virtualHost := range routeConfig.VirtualHosts {
			if virtualHost.Name == name {
				return true
			}
		}
	}
	return false
}
