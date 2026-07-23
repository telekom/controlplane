// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("AccessControl feature (kgateway)", func() {
	newBuilder := func(route *gatewayv1.Route) *Builder {
		gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns"}}
		b := NewFeatureBuilder(nil, route, nil, gw).(*Builder)
		b.SetUpstream(client.NewUpstreamOrDie("https://backend.svc.local:8080/api"))
		return b
	}

	routeWith := func(issuers, defaultConsumers []string, disable bool) *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				Security: gatewayv1.Security{
					TrustedIssuers:       issuers,
					DefaultConsumers:     defaultConsumers,
					DisableAccessControl: disable,
				},
			},
		}
	}

	It("IsUsed only when the route has trusted issuers", func() {
		Expect(InstanceAccessControlFeature.IsUsed(context.Background(), newBuilder(routeWith(nil, nil, false)))).To(BeFalse())
		Expect(InstanceAccessControlFeature.IsUsed(context.Background(), newBuilder(routeWith([]string{"https://iss"}, nil, false)))).To(BeTrue())
	})

	It("renders jwtAuth extensionRef + rbac allow-list from the Route", func() {
		route := routeWith([]string{"https://keycloak.example.com/auth/realms/p"}, []string{"kgateway-poc"}, false)
		b := newBuilder(route)
		Expect(InstanceAccessControlFeature.Apply(context.Background(), b)).To(Succeed())

		ext := b.buildJWTGatewayExtension()
		Expect(ext.Spec.JWT.Providers).To(HaveLen(1))
		Expect(ext.Spec.JWT.Providers[0].Issuer).To(Equal("https://keycloak.example.com/auth/realms/p"))
		Expect(ext.Spec.JWT.Providers[0].JWKS.RemoteJWKS.URL).To(Equal("https://keycloak.example.com/auth/realms/p/protocol/openid-connect/certs"))
		Expect(string(ext.Spec.JWT.Providers[0].JWKS.RemoteJWKS.BackendRef.Name)).To(Equal("jwks-keycloak.example.com"))

		tp := b.buildTrafficPolicy(ext.Name)
		Expect(string(tp.Spec.JWTAuth.ExtensionRef.Name)).To(Equal(ext.Name))
		Expect(tp.Spec.RBAC.Action).To(Equal(shared.AuthorizationPolicyActionAllow))
		Expect(tp.Spec.RBAC.Policy.MatchExpressions).To(ConsistOf(
			shared.CELExpression("metadata.filter_metadata['envoy.filters.http.jwt_authn']['payload']['azp'] == 'kgateway-poc'"),
		))
		Expect(tp.Spec.TargetRefs).To(HaveLen(1))
		Expect(string(tp.Spec.TargetRefs[0].Kind)).To(Equal("HTTPRoute"))
		Expect(string(tp.Spec.TargetRefs[0].Name)).To(Equal("my-route"))
	})

	It("empty allow-list yields deny-all (single false expression)", func() {
		route := routeWith([]string{"https://iss"}, nil, false)
		b := newBuilder(route)
		Expect(InstanceAccessControlFeature.Apply(context.Background(), b)).To(Succeed())

		tp := b.buildTrafficPolicy("ext")
		Expect(tp.Spec.RBAC.Policy.MatchExpressions).To(Equal([]shared.CELExpression{"false"}))
	})

	It("DisableAccessControl skips rbac but keeps jwtAuth", func() {
		route := routeWith([]string{"https://iss"}, []string{"c1"}, true)
		b := newBuilder(route)
		Expect(InstanceAccessControlFeature.Apply(context.Background(), b)).To(Succeed())

		tp := b.buildTrafficPolicy("ext")
		Expect(tp.Spec.JWTAuth).NotTo(BeNil())
		Expect(tp.Spec.RBAC).To(BeNil())
	})

	It("includes matching ConsumeRoutes in the allow-list, deduped with DefaultConsumers", func() {
		route := routeWith([]string{"https://iss"}, []string{"default-c"}, false)
		b := newBuilder(route)
		routeRef := ctypes.ObjectRef{Name: route.Name, Namespace: route.Namespace}
		b.AddAllowedConsumers(
			&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{Route: routeRef, ConsumerName: "cr-c"}},
			&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{Route: routeRef, ConsumerName: "default-c"}},
		)
		Expect(InstanceAccessControlFeature.Apply(context.Background(), b)).To(Succeed())
		Expect(b.intent.allowConsumers).To(Equal([]string{"default-c", "cr-c"}))
	})

	It("attaches an ExtensionRef filter to the HTTPRoute rule", func() {
		route := routeWith([]string{"https://iss"}, []string{"c1"}, false)
		b := newBuilder(route)
		Expect(InstanceAccessControlFeature.Apply(context.Background(), b)).To(Succeed())

		objs := b.buildResources()
		var hr *gwapiv1.HTTPRoute
		for _, o := range objs {
			if r, ok := o.(*gwapiv1.HTTPRoute); ok {
				hr = r
			}
		}
		Expect(hr).NotTo(BeNil())
		filters := hr.Spec.Rules[0].Filters
		Expect(filters).To(HaveLen(1))
		Expect(string(filters[0].ExtensionRef.Kind)).To(Equal("TrafficPolicy"))
		Expect(string(filters[0].ExtensionRef.Name)).To(Equal("my-route"))
	})
})
