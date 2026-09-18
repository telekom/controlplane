// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("RouteListener Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-routelistener"
			resourceNamespace = testEnvironment
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		var route *gatewayv1.Route

		BeforeEach(func() {
			createNamespace(resourceNamespace)

			By("creating a gateway for the route")
			gw := newGateway("rl-gateway", resourceNamespace)
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), &gatewayv1.Gateway{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, gw)).To(Succeed())
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			}

			By("creating the Route the RouteListener references")
			route = &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-route",
					Namespace: resourceNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: ctypes.ObjectRef{
						Name:      "rl-gateway",
						Namespace: resourceNamespace,
					},
					Type:      gatewayv1.RouteTypePrimary,
					Hostnames: []string{"api.test.com"},
					Paths:     []string{"/echo/v1"},
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{
							{Scheme: "https", Hostname: "backend.internal", Port: 443},
						},
					},
				},
			}
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(route), &gatewayv1.Route{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, route)).To(Succeed())
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				}, timeout, interval).Should(Succeed())
			}

			By("creating the custom resource for the Kind RouteListener")
			routelistener := &gatewayv1.RouteListener{}
			err = k8sClient.Get(ctx, typeNamespacedName, routelistener)
			if err != nil && errors.IsNotFound(err) {
				resource := &gatewayv1.RouteListener{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: gatewayv1.RouteListenerSpec{
						Route: ctypes.ObjectRef{
							Name:      "my-route",
							Namespace: resourceNamespace,
						},
						Zone: ctypes.ObjectRef{
							Name:      "aws",
							Namespace: resourceNamespace,
						},
						Consumer:     "eni--myteam--myapp",
						ServiceOwner: "eni--otherteam--provider",
						Issue:        "/echo/v1",
						GatewayClient: gatewayv1.GatewayClientConfig{
							ClientId: "gateway",
							Issuer:   "https://iris.example.com/auth/realms/default",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &gatewayv1.RouteListener{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance RouteListener")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Waiting for the RouteListener to become Ready via the controller")
			Eventually(func(g Gomega) {
				rl := &gatewayv1.RouteListener{}
				err := k8sClient.Get(ctx, typeNamespacedName, rl)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(rl.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should set Blocked condition when route does not exist", func() {
			By("Creating a RouteListener referencing a non-existent route")
			rl := &gatewayv1.RouteListener{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rl-no-route",
					Namespace: resourceNamespace,
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: gatewayv1.RouteListenerSpec{
					Route: ctypes.ObjectRef{
						Name:      "nonexistent-route",
						Namespace: resourceNamespace,
					},
					Zone: ctypes.ObjectRef{
						Name:      "aws",
						Namespace: resourceNamespace,
					},
					Consumer:     "eni--team--app",
					ServiceOwner: "eni--team--svc",
					Issue:        "/missing/v1",
					GatewayClient: gatewayv1.GatewayClientConfig{
						ClientId: "gw",
						Issuer:   "https://iris.example.com/auth/realms/default",
					},
				},
			}
			Expect(k8sClient.Create(ctx, rl)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rl), rl)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionFalse(rl.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			Expect(k8sClient.Delete(ctx, rl)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rl), rl)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should handle deletion gracefully", func() {
			By("Deleting the RouteListener")
			rl := &gatewayv1.RouteListener{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, rl)).To(Succeed())
			Expect(k8sClient.Delete(ctx, rl)).To(Succeed())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, rl)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
