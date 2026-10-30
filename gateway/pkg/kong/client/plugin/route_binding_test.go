// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type routeBoundPlugin interface {
	client.CustomPlugin
	client.RouteIdBinder
	client.RealmPlugin
}

var _ = Describe("Route binding", func() {
	var route *gatewayv1.Route

	BeforeEach(func() {
		route = &gatewayv1.Route{ObjectMeta: metav1.ObjectMeta{Name: "r"}}
		route.SetNamePrefix("bond")
		route.SetRouteId("route-id")
	})

	DescribeTable("binds route plugins by route ID and exposes the name prefix",
		func(build func(*gatewayv1.Route) routeBoundPlugin) {
			p := build(route)
			Expect(p.GetRouteId()).To(Equal("route-id"))
			Expect(p.GetNamePrefix()).To(Equal("bond"))
			Expect(*p.GetRoute()).To(Equal("r"))
		},
		Entry("acl", func(r *gatewayv1.Route) routeBoundPlugin { return AclPluginFromRoute(r) }),
		Entry("jwt", func(r *gatewayv1.Route) routeBoundPlugin { return JwtPluginFromRoute(r) }),
		Entry("request-termination", func(r *gatewayv1.Route) routeBoundPlugin { return RequestTerminationPluginFromRoute(r) }),
		Entry("request-transformer", func(r *gatewayv1.Route) routeBoundPlugin { return RequestTransformerPluginFromRoute(r) }),
		Entry("rate-limiting", func(r *gatewayv1.Route) routeBoundPlugin { return RateLimitPluginFromRoute(r) }),
	)

	It("binds the consumer rate-limiting plugin to the given route", func() {
		consumeRoute := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{ConsumerName: "c1"}}
		consumeRoute.Spec.Route.Name = "r"

		p := RateLimitPluginFromConsumeRoute(consumeRoute, route)
		Expect(p.GetRouteId()).To(Equal("route-id"))
		Expect(p.GetNamePrefix()).To(Equal("bond"))
		Expect(*p.GetConsumer()).To(Equal("c1"))

		unbound := RateLimitPluginFromConsumeRoute(consumeRoute, nil)
		Expect(unbound.GetRouteId()).To(BeEmpty())
		Expect(unbound.GetNamePrefix()).To(BeEmpty())
	})
})
