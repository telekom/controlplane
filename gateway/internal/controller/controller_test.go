// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

		It("should call CreateOrReplaceRoute with the correct route name, hostnames, and paths", func() {
			// Wait for the route to become Ready (proves the full pipeline succeeded)
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			// Inspect the mock's call log to verify the correct data was sent to Kong
			Eventually(func(g Gomega) {
				r, upstream, found := findRouteCall("my-api-route")
				g.Expect(found).To(BeTrue(), "CreateOrReplaceRoute should have been called with route 'my-api-route'")
				g.Expect(r.GetHostnames()).To(ContainElement("api.myservice.com"))
				g.Expect(r.GetPaths()).To(ContainElement("/payments/v1"))
				// LastMileSecurity sets upstream to localhost (Jumper proxy), not the backend directly
				g.Expect(upstream).NotTo(BeNil())
				g.Expect(upstream.GetHostname()).To(Equal("localhost"))
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
})
