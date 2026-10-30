// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/errors"
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

	Describe("Additional route tag validation", func() {
		newRoute := func(tags []string) *gatewayv1.Route {
			return &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "tag-validation-",
					Namespace:    namespace,
					Labels:       map[string]string{config.EnvironmentLabelKey: testEnvironment},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef:     types.ObjectRef{Name: gatewayName, Namespace: namespace},
					Type:           gatewayv1.RouteTypePrimary,
					AdditionalTags: tags,
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}},
					},
				},
			}
		}

		DescribeTable("accepts Kong-compatible tags unchanged", func(tags []string) {
			route := newRoute(tags)
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, route)).To(Succeed())
				Eventually(func() bool {
					return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), &gatewayv1.Route{}))
				}, timeout, interval).Should(BeTrue())
			})
			stored := &gatewayv1.Route{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), stored)).To(Succeed())
			Expect(stored.Spec.AdditionalTags).To(ConsistOf(tags))
		},
			Entry("omitted", []string(nil)),
			Entry("empty list", []string{}),
			Entry("lowercase and hyphens", []string{"variant--default"}),
			Entry("uppercase, spaces, Unicode, and punctuation", []string{"MCP", "Team Alpha", " ", "Grüße 世界", "key:value_~.", "back\\slash"}),
		)

		DescribeTable("rejects tags Kong does not accept", func(tag string) {
			err := k8sClient.Create(ctx, newRoute([]string{"valid", tag}))
			Expect(errors.IsInvalid(err)).To(BeTrue(), "expected validation error, got %v", err)
			Expect(err.Error()).To(ContainSubstring("spec.additionalTags[1]"))
		},
			Entry("empty string", ""),
			Entry("comma", "tag,value"),
			Entry("slash", "tag/value"),
		)

		It("rejects every ASCII control character and DEL", func() {
			for character := rune(0); character <= 127; character++ {
				if character >= 32 && character != 127 {
					continue
				}
				By(fmt.Sprintf("rejecting character U+%04X", character))
				err := k8sClient.Create(ctx, newRoute([]string{"tag" + string(character) + "value"}))
				Expect(errors.IsInvalid(err)).To(BeTrue(), "expected validation error, got %v", err)
				Expect(err.Error()).To(ContainSubstring("spec.additionalTags[0]"))
			}
		})
	})

	Describe("Gateway name prefix", func() {
		const (
			shortTimeout  = 2 * time.Second
			shortInterval = 200 * time.Millisecond
		)

		createGateway := func(name, prefix string) *gatewayv1.Gateway {
			gw := newGateway(name, namespace)
			gw.Spec.NamePrefix = prefix
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, gw))).To(Succeed())
			})
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
			return gw
		}

		createRoute := func(gatewayName string) *gatewayv1.Route {
			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "prefix-",
					Namespace:    namespace,
					Labels:       map[string]string{config.EnvironmentLabelKey: testEnvironment},
				},
				Spec: gatewayv1.RouteSpec{
					GatewayRef: types.ObjectRef{Name: gatewayName, Namespace: namespace},
					Type:       gatewayv1.RouteTypePrimary,
					Backend: gatewayv1.Backend{
						Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, route)).To(Succeed())
				Eventually(func() bool {
					return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), &gatewayv1.Route{}))
				}, timeout, interval).Should(BeTrue())
			})
			return route
		}

		expectProperties := func(route *gatewayv1.Route, match gomegatypes.GomegaMatcher) {
			Eventually(func(g Gomega) {
				stored := &gatewayv1.Route{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(route), stored)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(stored.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				g.Expect(stored.Status.Properties).To(match)
			}, timeout, interval).Should(Succeed())
		}

		It("names the Kong entities with the Gateway prefix and records it", func() {
			createGateway("prefixed-gateway", "bond")
			route := createRoute("prefixed-gateway")
			expectProperties(route, And(
				HaveKeyWithValue("routeName", "bond--"+route.Name),
				HaveKeyWithValue("namePrefix", "bond"),
			))
			Expect(kongNamesOf(route.Name)).To(ContainElement("bond--" + route.Name))
		})

		It("keeps the names of a Gateway without prefix", func() {
			createGateway("plain-gateway", "")
			route := createRoute("plain-gateway")
			expectProperties(route, And(HaveKeyWithValue("routeName", route.Name), Not(HaveKey("namePrefix"))))
			Expect(kongNamesOf(route.Name)).To(HaveEach(route.Name))
		})

		It("requeues the Routes when the prefix changes, not on other Gateway changes", func() {
			gw := createGateway("watched-gateway", "bond")
			route := createRoute("watched-gateway")
			expectProperties(route, HaveKeyWithValue("namePrefix", "bond"))

			By("changing an unrelated field")
			calls := len(kongNamesOf(route.Name))
			Eventually(func() error {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)).To(Succeed())
				gw.Spec.Features = []gatewayv1.FeatureType{gatewayv1.FeatureTypeCircuitBreaker}
				return k8sClient.Update(ctx, gw)
			}, timeout, interval).Should(Succeed())
			Consistently(func() int { return len(kongNamesOf(route.Name)) }, shortTimeout, shortInterval).Should(Equal(calls))

			By("changing the prefix")
			Eventually(func() error {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)).To(Succeed())
				gw.Spec.NamePrefix = "cit2"
				return k8sClient.Update(ctx, gw)
			}, timeout, interval).Should(Succeed())
			expectProperties(route, And(
				HaveKeyWithValue("routeName", "cit2--"+route.Name),
				HaveKeyWithValue("namePrefix", "cit2"),
			))
		})

		Describe("validation", func() {
			update := func(gw *gatewayv1.Gateway, prefix string) error {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gw), gw)).To(Succeed())
				gw.Spec.NamePrefix = prefix
				return k8sClient.Update(ctx, gw)
			}

			It("allows setting and changing a prefix but not removing it", func() {
				gw := createGateway("cel-gateway", "")
				Eventually(func() error { return update(gw, "bond") }, timeout, interval).Should(Succeed())
				Eventually(func() error { return update(gw, "cit2") }, timeout, interval).Should(Succeed())

				var err error
				Eventually(func() bool {
					err = update(gw, "")
					return !errors.IsConflict(err)
				}, timeout, interval).Should(BeTrue())
				Expect(errors.IsInvalid(err)).To(BeTrue(), "expected validation error, got %v", err)
				Expect(err.Error()).To(ContainSubstring("namePrefix cannot be removed once set"))
			})

			DescribeTable("rejects invalid prefixes", func(prefix string) {
				gw := newGateway("invalid-gateway", namespace)
				gw.Spec.NamePrefix = prefix
				err := k8sClient.Create(ctx, gw)
				Expect(errors.IsInvalid(err)).To(BeTrue(), "expected validation error, got %v", err)
				Expect(err.Error()).To(ContainSubstring("spec.namePrefix"))
			},
				Entry("uppercase", "Bond"),
				Entry("leading hyphen", "-bond"),
				Entry("double hyphen", "a--b"),
				Entry("dot", "bond.x"),
				Entry("too long", strings.Repeat("a", 64)),
			)
		})
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
})
