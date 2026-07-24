// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"sync"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("Gateway snapshot coordination", func() {
	ctx := context.Background()

	gateway := func(uid string) *gatewayv1.Gateway {
		return &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)}}
	}
	route := func(namespace, name, host string, port int) *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
			Spec: gatewayv1.RouteSpec{
				Hostnames: []string{name + ".example.com"},
				Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{
					Scheme: "https", Hostname: host, Port: int32(port),
				}}},
			},
		}
	}
	build := func(xds XdsClient, gw *gatewayv1.Gateway, r *gatewayv1.Route) error {
		return NewFeatureBuilder(xds, r, nil, gw).Build(ctx)
	}
	buildExpected := func(xds XdsClient, gw *gatewayv1.Gateway, r *gatewayv1.Route, expected ...*gatewayv1.Route) error {
		builder := NewFeatureBuilder(xds, r, nil, gw).(*Builder)
		ids := make([]string, 0, len(expected))
		for _, route := range expected {
			ids = append(ids, RouteIdentity(route))
		}
		builder.SetExpectedRouteIDs(ids)
		return builder.Build(ctx)
	}

	It("derives stable gateway node identities and rejects missing UIDs", func() {
		id, err := NodeIDForGateway(gateway("abc"))
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal("gateway:abc"))
		_, err = NodeIDForGateway(&gatewayv1.Gateway{})
		Expect(err).To(MatchError("gateway UID is empty"))
	})

	It("uses namespace-safe route-derived resource names", func() {
		a := route("team-a", "orders", "a.local", 8080)
		b := route("team-b", "orders", "b.local", 8080)
		Expect(routeResourceName(a)).To(Equal("route:team-a:orders"))
		Expect(routeResourceName(b)).To(Equal("route:team-b:orders"))
		Expect(routeResourceName(a)).NotTo(Equal(routeResourceName(b)))
	})

	It("combines two routes into one listener and unique clusters", func() {
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		gw := gateway("one")
		first, second := route("a", "first", "one.local", 8080), route("b", "second", "two.local", 9090)
		Expect(buildExpected(xds, gw, first, first, second)).To(Succeed())
		Expect(buildExpected(xds, gw, second, first, second)).To(Succeed())

		snapshot, err := cache.GetSnapshot("gateway:one")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.GetResources(resource.ListenerType)).To(HaveLen(1))
		Expect(snapshot.GetResources(resource.ClusterType)).To(HaveLen(2))
		Expect(snapshot.GetVersion(resource.ListenerType)).To(Equal(snapshot.GetVersion(resource.ClusterType)))
		listener := snapshot.GetResources(resource.ListenerType)["gateway-listener"].(*listenerv3.Listener)
		config, err := inlineRouteConfig(listener)
		Expect(err).NotTo(HaveOccurred())
		Expect(config.GetVirtualHosts()).To(HaveLen(2))
	})

	It("isolates gateways and supports route update, deletion, and gateway clear", func() {
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		one, two := gateway("one"), gateway("two")
		r1 := route("ns", "one", "one.local", 8080)
		r2 := route("ns", "two", "two.local", 8080)
		Expect(buildExpected(xds, one, r1, r1, r2)).To(Succeed())
		Expect(buildExpected(xds, one, r2, r1, r2)).To(Succeed())
		Expect(build(xds, two, route("other", "one", "isolated.local", 9090))).To(Succeed())

		r1.Spec.Backend.Upstreams[0].Hostname = "updated.local"
		Expect(buildExpected(xds, one, r1, r1, r2)).To(Succeed())
		Expect(xds.DeleteRoute(ctx, one, RouteIdentity(r2))).To(Succeed())
		snapshot, err := cache.GetSnapshot("gateway:one")
		Expect(err).NotTo(HaveOccurred())
		clusters := snapshot.GetResources(resource.ClusterType)
		Expect(clusters).To(HaveLen(1))
		cluster := clusters[routeResourceName(r1)].(*clusterv3.Cluster)
		address := cluster.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetSocketAddress()
		Expect(address.GetAddress()).To(Equal("updated.local"))
		other, err := cache.GetSnapshot("gateway:two")
		Expect(err).NotTo(HaveOccurred())
		Expect(other.GetResources(resource.ClusterType)).To(HaveLen(1))

		Expect(xds.ClearGateway(ctx, one)).To(Succeed())
		empty, err := cache.GetSnapshot("gateway:one")
		Expect(err).NotTo(HaveOccurred())
		Expect(empty.GetResources(resource.ClusterType)).To(BeEmpty())
		Expect(empty.GetResources(resource.ListenerType)).To(BeEmpty())
	})

	It("rejects incompatible route filter chains without replacing the valid state", func() {
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		gw := gateway("filters")
		plain := route("ns", "plain", "plain.local", 8080)
		secured := route("ns", "secured", "secured.local", 8080)
		Expect(buildExpected(xds, gw, plain, plain, secured)).To(Succeed())
		secured.Spec.Security.TrustedIssuers = []string{"https://issuer.example.com"}
		builder := NewFeatureBuilder(xds, secured, nil, gw)
		builder.(*Builder).SetExpectedRouteIDs([]string{RouteIdentity(plain), RouteIdentity(secured)})
		builder.EnableFeature(InstanceAccessControlFeature)
		Expect(builder.Build(ctx)).To(MatchError(ContainSubstring("HTTP filter chains differ")))
		_, err := cache.GetSnapshot("gateway:filters")
		Expect(err).To(HaveOccurred())
	})

	It("handles concurrent route publication without losing contributions", func() {
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		xds := NewXdsClient(cache)
		gw := gateway("concurrent")
		var wg sync.WaitGroup
		routes := make([]*gatewayv1.Route, 20)
		for i := range routes {
			routes[i] = route("ns", string(rune('a'+i)), "backend.local", 8080+i)
		}
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer GinkgoRecover()
				defer wg.Done()
				Expect(buildExpected(xds, gw, routes[i], routes...)).To(Succeed())
			}(i)
		}
		wg.Wait()
		snapshot, err := cache.GetSnapshot("gateway:concurrent")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.GetResources(resource.ClusterType)).To(HaveLen(20))
	})

	It("does not publish partial state while rebuilding after restart", func() {
		// arrange
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		gw := gateway("restart")
		first, second := route("ns", "first", "one.local", 8080), route("ns", "second", "two.local", 9090)
		xds := NewXdsClient(cache)

		// act
		Expect(buildExpected(xds, gw, first, first, second)).To(Succeed())
		_, err := cache.GetSnapshot("gateway:restart")

		// assert
		Expect(err).To(HaveOccurred())
		Expect(buildExpected(xds, gw, second, first, second)).To(Succeed())
		snapshot, err := cache.GetSnapshot("gateway:restart")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.GetResources(resource.ClusterType)).To(HaveLen(2))
	})

	It("does not publish deletion before surviving routes are rebuilt after restart", func() {
		// arrange
		cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
		gw := gateway("delete-restart")
		first, second := route("ns", "first", "one.local", 8080), route("ns", "second", "two.local", 9090)
		xds := NewXdsClient(cache)

		// act
		Expect(xds.DeleteRouteWithExpected(ctx, gw, RouteIdentity(first), []string{RouteIdentity(second)})).To(Succeed())
		_, err := cache.GetSnapshot("gateway:delete-restart")

		// assert
		Expect(err).To(HaveOccurred())
		Expect(buildExpected(xds, gw, second, second)).To(Succeed())
		snapshot, err := cache.GetSnapshot("gateway:delete-restart")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.GetResources(resource.ClusterType)).To(HaveLen(1))
	})
})

var _ = Describe("content-derived snapshot generation", func() {
	It("is stable across resource order and client restart", func() {
		first := ResourceBundle{Clusters: []*clusterv3.Cluster{{Name: "b"}, {Name: "a"}}}
		second := ResourceBundle{Clusters: []*clusterv3.Cluster{{Name: "a"}, {Name: "b"}}}
		one, err := generationFor(first)
		Expect(err).NotTo(HaveOccurred())
		two, err := generationFor(second)
		Expect(err).NotTo(HaveOccurred())
		Expect(two).To(Equal(one))
		Expect(NewXdsClient(cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil))).NotTo(BeNil())
		restarted, err := generationFor(first)
		Expect(err).NotTo(HaveOccurred())
		Expect(restarted).To(Equal(one))
	})

	It("changes when content changes", func() {
		one, err := generationFor(ResourceBundle{Clusters: []*clusterv3.Cluster{{Name: "a"}}})
		Expect(err).NotTo(HaveOccurred())
		two, err := generationFor(ResourceBundle{Clusters: []*clusterv3.Cluster{{Name: "a", AltStatName: "changed"}}})
		Expect(err).NotTo(HaveOccurred())
		Expect(two).NotTo(Equal(one))
	})
})
