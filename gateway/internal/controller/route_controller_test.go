// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RouteReconciler mapConsumeRouteToRoute", Ordered, func() {
	const (
		routeName      = "cross-zone-api-v1"
		zoneA          = "map-zone-a"
		zoneB          = "map-zone-b"
		zoneC          = "map-zone-c"
		zoneD          = "map-zone-d"
		otherEnvZone   = "map-other-env"
		otherEnvLabel  = "other"
		gatewayName    = "map-gateway"
		consumerPrefix = "map-consumer"
	)

	var reconciler *RouteReconciler
	var gateways []*gatewayv1.Gateway
	var created []client.Object

	gatewayNamespace := map[string]string{testEnvironment: zoneA, otherEnvLabel: otherEnvZone}

	newRoute := func(namespace, env string, routeType gatewayv1.RouteType) *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeName,
				Namespace: namespace,
				Labels:    map[string]string{config.EnvironmentLabelKey: env},
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: types.ObjectRef{Name: gatewayName, Namespace: gatewayNamespace[env]},
				Type:       routeType,
				Backend: gatewayv1.Backend{
					Upstreams: []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}},
				},
			},
		}
	}

	newConsumeRoute := func(name string, labels map[string]string) *gatewayv1.ConsumeRoute {
		return &gatewayv1.ConsumeRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: zoneB,
				Labels:    labels,
			},
			Spec: gatewayv1.ConsumeRouteSpec{
				Route:        types.ObjectRef{Name: routeName, Namespace: zoneB},
				ConsumerName: "subscriber-app",
			},
		}
	}

	request := func(namespace string) reconcile.Request {
		return reconcile.Request{NamespacedName: client.ObjectKey{Name: routeName, Namespace: namespace}}
	}

	BeforeAll(func() {
		reconciler = &RouteReconciler{Client: mgrClient}

		for _, ns := range []string{zoneA, zoneB, zoneC, zoneD, otherEnvZone} {
			createNamespace(ns)
		}

		// Routes need a ready Gateway of their environment, otherwise their finalizer blocks deletion.
		for env, ns := range gatewayNamespace {
			gateway := newGateway(gatewayName, ns)
			gateway.Labels[config.EnvironmentLabelKey] = env
			Expect(k8sClient.Create(ctx, gateway)).To(Succeed())
			gateways = append(gateways, gateway)
		}
		for _, gateway := range gateways {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(gateway.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		}

		created = []client.Object{
			newRoute(zoneA, testEnvironment, gatewayv1.RouteTypePrimary),
			newRoute(zoneB, testEnvironment, gatewayv1.RouteTypeProxy),
			newRoute(zoneC, testEnvironment, gatewayv1.RouteTypeProxy),
			newRoute(zoneD, testEnvironment, gatewayv1.RouteTypeSecondary),
			newRoute(otherEnvZone, otherEnvLabel, gatewayv1.RouteTypePrimary),
		}
		for _, obj := range created {
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
		}
	})

	AfterAll(func() {
		for _, obj := range created {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed())
		}
		for _, obj := range created {
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), &gatewayv1.Route{}))
			}, timeout, interval).Should(BeTrue())
		}
		for _, gateway := range gateways {
			Expect(k8sClient.Delete(ctx, gateway)).To(Succeed())
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), &gatewayv1.Gateway{}))
			}, timeout, interval).Should(BeTrue())
		}
	})

	It("enqueues the direct Route and same-named primary and failover Routes of the same environment", func() {
		consumeRoute := newConsumeRoute(consumerPrefix+"-env", map[string]string{config.EnvironmentLabelKey: testEnvironment})

		Eventually(func(g Gomega) {
			g.Expect(reconciler.mapConsumeRouteToRoute(ctx, consumeRoute)).To(ConsistOf(
				request(zoneB),
				request(zoneA),
				request(zoneD),
			))
		}, timeout, interval).Should(Succeed())
	})

	It("enqueues only the direct Route when the ConsumeRoute has no environment label", func() {
		consumeRoute := newConsumeRoute(consumerPrefix+"-no-env", nil)

		Expect(reconciler.mapConsumeRouteToRoute(ctx, consumeRoute)).To(ConsistOf(request(zoneB)))
	})

	It("enqueues the direct Route even if it does not exist", func() {
		consumeRoute := newConsumeRoute(consumerPrefix+"-missing", map[string]string{config.EnvironmentLabelKey: testEnvironment})
		consumeRoute.Spec.Route = types.ObjectRef{Name: "missing-route", Namespace: zoneB}

		Expect(reconciler.mapConsumeRouteToRoute(ctx, consumeRoute)).To(ConsistOf(
			reconcile.Request{NamespacedName: client.ObjectKey{Name: "missing-route", Namespace: zoneB}},
		))
	})
})
