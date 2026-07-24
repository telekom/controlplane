// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy_test

import (
	"context"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	rbacconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	rbacfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	"github.com/telekom/controlplane/gateway/internal/features/envoy/feature"
)

var _ = Describe("AccessControl feature", func() {
	var (
		ctx   context.Context
		xds   *envoy.XdsCache
		cache cachev3.SnapshotCache
	)

	const upstreamHTTP = "http"

	BeforeEach(func() {
		ctx = context.Background()
		xds = envoy.NewXdsCache(GinkgoLogr)
		var ok bool
		cache, ok = xds.Cache().(cachev3.SnapshotCache)
		Expect(ok).To(BeTrue())
	})

	buildWith := func(route *gatewayv1.Route, consumers ...*gatewayv1.ConsumeRoute) *hcmv3.HttpConnectionManager {
		b := envoy.NewEnvoyFeatureBuilder(xds, route, nil, &gatewayv1.Gateway{})
		b.AddAllowedConsumers(consumers...)
		b.EnableFeature(feature.InstanceAccessControlFeature)
		Expect(b.Build(ctx)).To(Succeed())

		snap, err := cache.GetSnapshot(route.Spec.GatewayRef.Name)
		Expect(err).NotTo(HaveOccurred())
		for _, r := range snap.GetResources(resourcev3.ListenerType) {
			l := r.(*listenerv3.Listener)
			hcm := &hcmv3.HttpConnectionManager{}
			Expect(l.GetFilterChains()[0].GetFilters()[0].GetTypedConfig().UnmarshalTo(hcm)).To(Succeed())
			return hcm
		}
		Fail("no listener in snapshot")
		return nil
	}

	filterNames := func(hcm *hcmv3.HttpConnectionManager) []string {
		var names []string
		for _, f := range hcm.GetHttpFilters() {
			names = append(names, f.GetName())
		}
		return names
	}

	unmarshalRBAC := func(hcm *hcmv3.HttpConnectionManager) *rbacconfigv3.RBAC {
		for _, f := range hcm.GetHttpFilters() {
			if f.GetName() == "envoy.filters.http.rbac" {
				wrapper := &rbacfilterv3.RBAC{}
				Expect(f.GetTypedConfig().UnmarshalTo(wrapper)).To(Succeed())
				return wrapper.GetRules()
			}
		}
		Fail("no rbac filter")
		return nil
	}

	unmarshalJWT := func(hcm *hcmv3.HttpConnectionManager) *jwtauthnv3.JwtAuthentication {
		for _, f := range hcm.GetHttpFilters() {
			if f.GetName() == "envoy.filters.http.jwt_authn" {
				cfg := &jwtauthnv3.JwtAuthentication{}
				Expect(f.GetTypedConfig().UnmarshalTo(cfg)).To(Succeed())
				return cfg
			}
		}
		Fail("no jwt_authn filter")
		return nil
	}

	acRoute := func(issuers, defaultConsumers []string, disable bool) *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns"},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: types.ObjectRef{Name: "my-gw"},
				Paths:      []string{"/"},
				Backend:    gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{Scheme: upstreamHTTP, Hostname: "backend", Port: 8080}}},
				Security: gatewayv1.Security{
					TrustedIssuers:       issuers,
					DefaultConsumers:     defaultConsumers,
					DisableAccessControl: disable,
					RealmName:            "realm",
				},
			},
		}
	}

	consumeRoute := func(consumerName string) *gatewayv1.ConsumeRoute {
		return &gatewayv1.ConsumeRoute{
			Spec: gatewayv1.ConsumeRouteSpec{
				Route:        types.ObjectRef{Name: "my-route", Namespace: "ns"},
				ConsumerName: consumerName,
			},
		}
	}

	It("does not add auth filters when the route has no trusted issuers", func() {
		hcm := buildWith(acRoute(nil, nil, false))
		Expect(filterNames(hcm)).To(Equal([]string{"envoy.filters.http.router"}))
	})

	It("adds jwt_authn and rbac before the router, in order", func() {
		hcm := buildWith(acRoute([]string{"https://kc/realms/a"}, []string{"c1"}, false))
		Expect(filterNames(hcm)).To(Equal([]string{
			"envoy.filters.http.jwt_authn",
			"envoy.filters.http.rbac",
			"envoy.filters.http.router",
		}))
	})

	It("creates one jwt provider per trusted issuer, all requires_any", func() {
		hcm := buildWith(acRoute([]string{"https://kc/realms/a", "https://kc/realms/b"}, []string{"c1"}, false))
		jwt := unmarshalJWT(hcm)
		Expect(jwt.GetProviders()).To(HaveLen(2))
		reqs := jwt.GetRules()[0].GetRequires().GetRequiresAny().GetRequirements()
		Expect(reqs).To(HaveLen(2))
	})

	It("allow-lists default consumers plus route-matched allowed consumers", func() {
		hcm := buildWith(
			acRoute([]string{"https://kc/realms/a"}, []string{"default-c"}, false),
			consumeRoute("allowed-c"),
			// belongs to a different route -> excluded
			&gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
				Route: types.ObjectRef{Name: "other-route", Namespace: "ns"}, ConsumerName: "excluded-c",
			}},
		)
		rbac := unmarshalRBAC(hcm)
		Expect(rbac.GetAction()).To(Equal(rbacconfigv3.RBAC_ALLOW))
		principals := rbac.GetPolicies()["allow-consumers"].GetPrincipals()

		var matched []string
		for _, p := range principals {
			matched = append(matched, p.GetMetadata().GetValue().GetStringMatch().GetExact())
		}
		Expect(matched).To(ConsistOf("default-c", "allowed-c"))

		// path is [payload key, azp]
		seg := principals[0].GetMetadata().GetPath()
		Expect(seg).To(HaveLen(2))
		Expect(seg[1].GetKey()).To(Equal("azp"))
	})

	It("empty allow-list yields an ALLOW rbac with no policies (deny all)", func() {
		hcm := buildWith(acRoute([]string{"https://kc/realms/a"}, nil, false))
		rbac := unmarshalRBAC(hcm)
		Expect(rbac.GetAction()).To(Equal(rbacconfigv3.RBAC_ALLOW))
		Expect(rbac.GetPolicies()).To(BeEmpty())
	})

	It("skips rbac when access control is disabled but keeps jwt_authn", func() {
		hcm := buildWith(acRoute([]string{"https://kc/realms/a"}, []string{"c1"}, true))
		Expect(filterNames(hcm)).To(Equal([]string{
			"envoy.filters.http.jwt_authn",
			"envoy.filters.http.router",
		}))
	})
})
