// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	httprbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CompileGateway", func() {
	var aggregate GatewayAggregate

	BeforeEach(func() {
		gateway := &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gateway", Namespace: "team-a", UID: types.UID("gateway-uid"), Generation: 3,
			},
			Spec: gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy},
		}
		routeA := gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "team-a", UID: types.UID("route-a"), Generation: 4},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: gateway.Name, Namespace: gateway.Namespace},
				Hostnames:  []string{"orders.example.com"}, Paths: []string{"/v1"},
				Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{
					Scheme: "http", Hostname: "orders.internal", Port: 8080,
				}}},
				Security: gatewayv1.Security{
					TrustedIssuers: []string{"https://issuer-b", "https://issuer-a"},
				},
			},
		}
		routeB := gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "catalog", Namespace: "team-b", UID: types.UID("route-b"), Generation: 2},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: ctypes.ObjectRef{Name: gateway.Name, Namespace: gateway.Namespace},
				Hostnames:  []string{"catalog.example.com"}, Paths: []string{"/catalog"},
				Backend: gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{{
					Scheme: "http", Hostname: "catalog.internal", Port: 9090,
				}}},
			},
		}
		aggregate = GatewayAggregate{
			Environment: "test", Gateway: gateway, Routes: []gatewayv1.Route{routeA, routeB},
			Consumers: []gatewayv1.Consumer{{
				ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "team-a", Generation: 1},
				Spec: gatewayv1.ConsumerSpec{
					Gateway: ctypes.ObjectRef{Name: gateway.Name, Namespace: gateway.Namespace}, Name: "client-a",
				},
			}},
			ConsumeRoutes: []gatewayv1.ConsumeRoute{{
				ObjectMeta: metav1.ObjectMeta{Name: "subscription", Namespace: "team-a", Generation: 1},
				Spec: gatewayv1.ConsumeRouteSpec{
					Route: ctypes.ObjectRef{Name: routeA.Name, Namespace: routeA.Namespace}, ConsumerName: "client-a",
				},
			}},
		}
	})

	It("renders a deterministic namespace-safe complete LDS/RDS/CDS/EDS bundle", func() {
		first, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())
		aggregate.Routes[0], aggregate.Routes[1] = aggregate.Routes[1], aggregate.Routes[0]
		second, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())

		Expect(first.Target).To(Equal(TargetIdentity{
			Environment: "test", Namespace: "team-a", Name: "gateway", UID: types.UID("gateway-uid"),
		}))
		Expect(first.Listeners).To(HaveLen(1))
		Expect(first.Routes).To(HaveLen(1))
		Expect(first.Routes[0].VirtualHosts).To(HaveLen(2))
		Expect(first.Endpoints).To(HaveLen(2))
		Expect(endpointNames(first.Endpoints)).To(Equal([]string{"team-a/orders", "team-b/catalog"}))
		Expect(first.Source.Resources).To(HaveLen(5))
		Expect(first.Source.Resources[0].Kind).To(Equal("ConsumeRoute"))
		Expect(first.Target).To(Equal(second.Target))
		Expect(first.Source).To(Equal(second.Source))
		Expect(protoSlicesEqual(first.Listeners, second.Listeners)).To(BeTrue())
		Expect(first.Routes[0]).To(Equal(second.Routes[0]))
		Expect(protoSlicesEqual(first.Clusters, second.Clusters)).To(BeTrue())
		Expect(protoSlicesEqual(first.Endpoints, second.Endpoints)).To(BeTrue())

		snapshot, err := cachev3.NewSnapshot("1", map[resource.Type][]cachetypes.Resource{
			resource.ListenerType: toResources(first.Listeners), resource.RouteType: toResources(first.Routes),
			resource.ClusterType: toResources(first.Clusters), resource.EndpointType: toResources(first.Endpoints),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.Consistent()).To(Succeed())
	})

	It("reflects updates and removes omitted route resources", func() {
		created, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())

		aggregate.Routes[0].Spec.Paths = []string{"/v2"}
		updated, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())
		Expect(findVirtualHost(&updated, "team-a/orders").Routes[0].GetMatch().GetPrefix()).To(Equal("/v2"))
		Expect(bundlesEqual(&created, &updated)).To(BeFalse())

		aggregate.Routes = aggregate.Routes[1:]
		deleted, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())
		Expect(findVirtualHost(&deleted, "team-a/orders")).To(BeNil())
		Expect(clusterNames(deleted.Clusters)).NotTo(ContainElement("team-a/orders"))
		Expect(endpointNames(deleted.Endpoints)).NotTo(ContainElement("team-a/orders"))
	})

	It("renders a valid not-found listener when no routes remain", func() {
		aggregate.Routes = nil
		aggregate.ConsumeRoutes = nil

		bundle, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())
		Expect(bundle.Source.Resources).To(HaveLen(2))
		Expect(bundle.Listeners).To(HaveLen(1))
		Expect(bundle.Routes).To(HaveLen(1))
		Expect(bundle.Routes[0].VirtualHosts).To(HaveLen(1))
		Expect(bundle.Routes[0].VirtualHosts[0].Routes[0].GetDirectResponse().Status).To(Equal(uint32(404)))
		Expect(bundle.Clusters).To(BeEmpty())
		Expect(bundle.Endpoints).To(BeEmpty())
	})

	It("keeps aggregate RBAC inert globally and configures it per protected route", func() {
		bundle, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).NotTo(HaveOccurred())
		protected := findVirtualHost(&bundle, "team-a/orders")
		unprotected := findVirtualHost(&bundle, "team-b/catalog")
		Expect(protected.TypedPerFilterConfig).To(HaveKey(filterRBAC))
		Expect(unprotected.TypedPerFilterConfig).NotTo(HaveKey(filterRBAC))

		filters, err := aggregateFilters(map[string]accessControlIntent{
			"team-a/orders": {trustedIssuers: []string{"https://issuer-a"}, accessControl: true},
		}, false)
		Expect(err).NotTo(HaveOccurred())
		globalRBAC := &httprbacv3.RBAC{}
		Expect(filterByCanonicalName(filters, filterRBAC).GetTypedConfig().UnmarshalTo(globalRBAC)).To(Succeed())
		Expect(globalRBAC.Rules).To(BeNil())
	})

	It("rejects conflicting virtual-host domains before publication", func() {
		aggregate.Routes[1].Spec.Hostnames = aggregate.Routes[0].Spec.Hostnames
		_, err := CompileGateway(context.Background(), &aggregate)
		Expect(err).To(MatchError(ContainSubstring("conflicting domain")))
	})
})

func clusterNames(resources []*clusterv3.Cluster) []string {
	names := make([]string, 0, len(resources))
	for _, item := range resources {
		names = append(names, item.Name)
	}
	return names
}

func endpointNames(resources []*endpointv3.ClusterLoadAssignment) []string {
	names := make([]string, 0, len(resources))
	for _, item := range resources {
		names = append(names, item.ClusterName)
	}
	return names
}

func bundlesEqual(a, b *ResourceBundle) bool {
	if a.Target != b.Target || len(a.Source.Resources) != len(b.Source.Resources) ||
		len(a.Listeners) != len(b.Listeners) || len(a.Routes) != len(b.Routes) ||
		len(a.Clusters) != len(b.Clusters) || len(a.Endpoints) != len(b.Endpoints) {
		return false
	}
	for i := range a.Source.Resources {
		if a.Source.Resources[i] != b.Source.Resources[i] {
			return false
		}
	}
	return proto.Equal(a.Listeners[0], b.Listeners[0]) && proto.Equal(a.Routes[0], b.Routes[0]) &&
		protoSlicesEqual(a.Clusters, b.Clusters) && protoSlicesEqual(a.Endpoints, b.Endpoints)
}

func protoSlicesEqual[T proto.Message](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func findVirtualHost(bundle *ResourceBundle, name string) *routev3.VirtualHost {
	for _, virtualHost := range bundle.Routes[0].VirtualHosts {
		if virtualHost.Name == name {
			return virtualHost
		}
	}
	return nil
}
