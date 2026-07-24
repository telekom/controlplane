// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy_test

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
)

func makeRoute(name, gatewayName string, hostnames, paths []string, upstreams ...gatewayv1.Upstream) *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: gatewayv1.RouteSpec{
			GatewayRef: types.ObjectRef{Name: gatewayName},
			Hostnames:  hostnames,
			Paths:      paths,
			Backend:    gatewayv1.Backend{Upstreams: upstreams},
		},
	}
}

var _ = Describe("Builder.Build core routing", func() {
	var (
		ctx   context.Context
		xds   *envoy.XdsCache
		cache cachev3.SnapshotCache
	)

	BeforeEach(func() {
		ctx = context.Background()
		xds = envoy.NewXdsCache(GinkgoLogr)
		var ok bool
		cache, ok = xds.Cache().(cachev3.SnapshotCache)
		Expect(ok).To(BeTrue())
	})

	build := func(route *gatewayv1.Route) features.EnvoyFeatureBuilder {
		b := envoy.NewEnvoyFeatureBuilder(xds, route, nil, &gatewayv1.Gateway{})
		Expect(b.Build(ctx)).To(Succeed())
		return b
	}

	snapshotFor := func(gatewayName string) cachev3.ResourceSnapshot {
		snap, err := cache.GetSnapshot(gatewayName)
		Expect(err).NotTo(HaveOccurred())
		return snap
	}

	It("errors when the route has no upstream", func() {
		route := makeRoute("r", "gw", nil, nil)
		b := envoy.NewEnvoyFeatureBuilder(xds, route, nil, &gatewayv1.Gateway{})
		Expect(b.Build(ctx)).NotTo(Succeed())
	})

	It("publishes listener, routeconfig and cluster keyed on the gateway", func() {
		route := makeRoute("my-route", "my-gw",
			[]string{"api.example.com"}, []string{"/api"},
			gatewayv1.Upstream{Scheme: "http", Hostname: "backend", Port: 8080})
		build(route)

		snap := snapshotFor("my-gw")
		Expect(snap.GetResources(resourcev3.ListenerType)).To(HaveLen(1))
		Expect(snap.GetResources(resourcev3.RouteType)).To(HaveLen(1))
		Expect(snap.GetResources(resourcev3.ClusterType)).To(HaveLen(1))
	})

	It("maps hostnames to vhost domains and paths to prefix routes", func() {
		route := makeRoute("my-route", "my-gw",
			[]string{"api.example.com", "www.example.com"}, []string{"/v1", "/v2"},
			gatewayv1.Upstream{Scheme: "http", Hostname: "backend", Port: 8080})
		build(route)

		rc := unmarshalRouteConfig(snapshotFor("my-gw"))
		Expect(rc.VirtualHosts).To(HaveLen(1))
		Expect(rc.VirtualHosts[0].Domains).To(ConsistOf("api.example.com", "www.example.com"))

		prefixes := []string{}
		for _, r := range rc.VirtualHosts[0].Routes {
			prefixes = append(prefixes, r.GetMatch().GetPrefix())
			Expect(r.GetRoute().GetCluster()).To(Equal("my-route-cluster"))
		}
		Expect(prefixes).To(ConsistOf("/v1", "/v2"))
	})

	It("defaults empty hostnames to * and empty paths to /", func() {
		route := makeRoute("my-route", "my-gw", nil, nil,
			gatewayv1.Upstream{Scheme: "http", Hostname: "backend", Port: 8080})
		build(route)

		rc := unmarshalRouteConfig(snapshotFor("my-gw"))
		Expect(rc.VirtualHosts[0].Domains).To(ConsistOf("*"))
		Expect(rc.VirtualHosts[0].Routes).To(HaveLen(1))
		Expect(rc.VirtualHosts[0].Routes[0].GetMatch().GetPrefix()).To(Equal("/"))
	})

	It("wires the HCM to RDS via ADS with the router filter", func() {
		route := makeRoute("my-route", "my-gw", nil, []string{"/"},
			gatewayv1.Upstream{Scheme: "http", Hostname: "backend", Port: 8080})
		build(route)

		hcm := unmarshalHCM(snapshotFor("my-gw"))
		Expect(hcm.GetRds().GetRouteConfigName()).To(Equal("my-route-routes"))
		Expect(hcm.GetRds().GetConfigSource().GetAds()).NotTo(BeNil())
		Expect(hcm.GetHttpFilters()).To(HaveLen(1))
		Expect(hcm.GetHttpFilters()[0].GetName()).To(Equal("envoy.filters.http.router"))
	})

	It("builds a STRICT_DNS cluster to the upstream target without TLS for http", func() {
		route := makeRoute("my-route", "my-gw", nil, []string{"/"},
			gatewayv1.Upstream{Scheme: "http", Hostname: "backend", Port: 8080})
		build(route)

		c := unmarshalCluster(snapshotFor("my-gw"))
		Expect(c.GetType()).To(Equal(clusterv3.Cluster_STRICT_DNS))
		sa := c.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].
			GetEndpoint().GetAddress().GetSocketAddress()
		Expect(sa.GetAddress()).To(Equal("backend"))
		Expect(sa.GetPortValue()).To(Equal(uint32(8080)))
		Expect(c.GetTransportSocket()).To(BeNil())
	})

	It("adds a TLS transport socket for https upstreams", func() {
		route := makeRoute("my-route", "my-gw", nil, []string{"/"},
			gatewayv1.Upstream{Scheme: "https", Hostname: "secure.backend", Port: 443})
		build(route)

		c := unmarshalCluster(snapshotFor("my-gw"))
		Expect(c.GetTransportSocket()).NotTo(BeNil())
		Expect(c.GetTransportSocket().GetName()).To(Equal("envoy.transport_sockets.tls"))
	})
})

func unmarshalRouteConfig(snap cachev3.ResourceSnapshot) *routev3.RouteConfiguration {
	for _, r := range snap.GetResources(resourcev3.RouteType) {
		return r.(*routev3.RouteConfiguration)
	}
	Fail("no route configuration in snapshot")
	return nil
}

func unmarshalCluster(snap cachev3.ResourceSnapshot) *clusterv3.Cluster {
	for _, r := range snap.GetResources(resourcev3.ClusterType) {
		return r.(*clusterv3.Cluster)
	}
	Fail("no cluster in snapshot")
	return nil
}

func unmarshalHCM(snap cachev3.ResourceSnapshot) *hcmv3.HttpConnectionManager {
	for _, r := range snap.GetResources(resourcev3.ListenerType) {
		l := r.(*listenerv3.Listener)
		filter := l.GetFilterChains()[0].GetFilters()[0]
		hcm := &hcmv3.HttpConnectionManager{}
		Expect(filter.GetTypedConfig().UnmarshalTo(hcm)).To(Succeed())
		return hcm
	}
	Fail("no listener in snapshot")
	return nil
}
