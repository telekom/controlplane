// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
	"github.com/telekom/controlplane/gateway/internal/xds/storage"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func validBundle() *xdsapi.Bundle {
	hcm, err := anypb.New(&hcmv3.HttpConnectionManager{
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcmv3.HttpConnectionManager_Rds{Rds: &hcmv3.Rds{
			RouteConfigName: "route-a",
			ConfigSource: &corev3.ConfigSource{
				ResourceApiVersion:    corev3.ApiVersion_V3,
				ConfigSourceSpecifier: &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}},
			},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	listener, err := anypb.New(&listenerv3.Listener{
		Name: "listener-a",
		FilterChains: []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{{
			Name:       "envoy.filters.network.http_connection_manager",
			ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: hcm},
		}}}},
	})
	Expect(err).NotTo(HaveOccurred())
	route, err := anypb.New(&routev3.RouteConfiguration{
		Name: "route-a",
		VirtualHosts: []*routev3.VirtualHost{{
			Name: "virtual-host-a", Domains: []string{"*"},
			Routes: []*routev3.Route{{
				Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"}},
				Action: &routev3.Route_Route{Route: &routev3.RouteAction{
					ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "cluster-a"},
				}},
			}},
		}},
	})
	Expect(err).NotTo(HaveOccurred())
	cluster, err := anypb.New(&clusterv3.Cluster{
		Name:                 "cluster-a",
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_EDS},
		EdsClusterConfig: &clusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &corev3.ConfigSource{
				ResourceApiVersion:    corev3.ApiVersion_V3,
				ConfigSourceSpecifier: &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}},
			},
			ServiceName: "endpoint-a",
		},
	})
	Expect(err).NotTo(HaveOccurred())
	endpoint, err := anypb.New(&endpointv3.ClusterLoadAssignment{ClusterName: "endpoint-a"})
	Expect(err).NotTo(HaveOccurred())
	bundle := &xdsapi.Bundle{
		TargetId: "target-a", PublisherGeneration: "1",
		SchemaVersion: xdsapi.SchemaVersion, CompilerVersion: "compiler-a", EnvoyVersion: "1.37",
		Sources:   []*xdsapi.SourceReference{{Kind: "Gateway", Name: "gateway-a"}},
		Listeners: []*anypb.Any{listener}, Routes: []*anypb.Any{route},
		Clusters: []*anypb.Any{cluster}, Endpoints: []*anypb.Any{endpoint},
	}
	Expect(xdsapi.SetDigest(bundle)).To(Succeed())
	return bundle
}

type failingCache struct {
	cachev3.SnapshotCache
	fail bool
}

func (c *failingCache) SetSnapshot(ctx context.Context, node string, snapshot cachev3.ResourceSnapshot) error {
	if c.fail {
		return fmt.Errorf("injected cache failure")
	}
	return c.SnapshotCache.SetSnapshot(ctx, node, snapshot)
}

type observationRecorder struct {
	mu           sync.Mutex
	observations []*xdsapi.DeliveryObservation
}

func (r *observationRecorder) RecordObservation(
	_ context.Context,
	_ string,
	observation *xdsapi.DeliveryObservation,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, observation)
	return nil
}

var _ = Describe("Validation", func() {
	It("accepts only named, unique LDS/RDS/CDS/EDS resources", func() {
		bundle := validBundle()
		snapshot, validationErrors := Validate(bundle, "1")
		Expect(validationErrors).To(BeEmpty())
		Expect(snapshot.Consistent()).To(Succeed())

		bundle.Listeners = append(bundle.Listeners, bundle.Listeners[0])
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())
		_, validationErrors = Validate(bundle, "2")
		Expect(validationErrors).To(ContainElement(HaveField("Code",
			xdsapi.ValidationCode_VALIDATION_CODE_DUPLICATE_RESOURCE)))
	})

	It("rejects digest mismatch without constructing a snapshot", func() {
		bundle := validBundle()
		bundle.Digest = "tampered"
		snapshot, validationErrors := Validate(bundle, "1")
		Expect(snapshot).To(BeNil())
		Expect(validationErrors).To(ContainElement(HaveField("Code",
			xdsapi.ValidationCode_VALIDATION_CODE_DIGEST_MISMATCH)))
	})

	It("rejects resources that violate Envoy protobuf validation constraints", func() {
		bundle := validBundle()
		invalid, err := anypb.New(&clusterv3.Cluster{})
		Expect(err).NotTo(HaveOccurred())
		bundle.Clusters = []*anypb.Any{invalid}
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())

		_, validationErrors := Validate(bundle, "2")
		Expect(validationErrors).To(ContainElement(HaveField("Code",
			xdsapi.ValidationCode_VALIDATION_CODE_MALFORMED_RESOURCE)))
	})

	It("constructs an explicit empty snapshot for target deactivation", func() {
		bundle := validBundle()
		bundle.Listeners = nil
		bundle.Routes = nil
		bundle.Clusters = nil
		bundle.Endpoints = nil
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())

		snapshot, validationErrors := Validate(bundle, "2")
		Expect(validationErrors).To(BeEmpty())
		for _, typeURL := range supportedTypeURLs {
			Expect(snapshot.GetResourcesAndTTL(typeURL)).To(BeEmpty())
			Expect(snapshot.GetVersion(typeURL)).To(Equal("2"))
		}
	})

	It("rejects secret-bearing configuration", func() {
		bundle := validBundle()
		secret, err := anypb.New(&tlsv3.UpstreamTlsContext{CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{{PrivateKey: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{InlineString: "private"},
			}}},
		}})
		Expect(err).NotTo(HaveOccurred())
		cluster := &clusterv3.Cluster{}
		Expect(anypb.UnmarshalTo(bundle.Clusters[0], cluster, proto.UnmarshalOptions{})).To(Succeed())
		cluster.TransportSocket = &corev3.TransportSocket{
			Name: "envoy.transport_sockets.tls", ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: secret},
		}
		bundle.Clusters[0], err = anypb.New(cluster)
		Expect(err).NotTo(HaveOccurred())
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())

		_, validationErrors := Validate(bundle, "2")
		Expect(validationErrors).To(ContainElement(HaveField("Message", "secret-bearing xDS configuration is not supported")))
	})

	It("rejects registered protobuf types in unsupported nested Any fields", func() {
		bundle := validBundle()
		listener := &listenerv3.Listener{}
		Expect(anypb.UnmarshalTo(bundle.Listeners[0], listener, proto.UnmarshalOptions{})).To(Succeed())
		manager := &hcmv3.HttpConnectionManager{}
		Expect(listener.FilterChains[0].Filters[0].GetTypedConfig().UnmarshalTo(manager)).To(Succeed())
		wrongType, err := anypb.New(&corev3.Node{Id: "registered-but-not-an-http-filter"})
		Expect(err).NotTo(HaveOccurred())
		manager.HttpFilters = []*hcmv3.HttpFilter{{
			Name: "invalid", ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: wrongType},
		}}
		managerAny, err := anypb.New(manager)
		Expect(err).NotTo(HaveOccurred())
		listener.FilterChains[0].Filters[0].ConfigType = &listenerv3.Filter_TypedConfig{TypedConfig: managerAny}
		bundle.Listeners[0], err = anypb.New(listener)
		Expect(err).NotTo(HaveOccurred())
		Expect(xdsapi.SetDigest(bundle)).To(Succeed())

		_, validationErrors := Validate(bundle, "2")
		Expect(validationErrors).To(ContainElement(HaveField("Message",
			"nested xDS extension violates Envoy validation constraints")))
	})
})

var _ = Describe("Mapping and callbacks", func() {
	It("maps authorized nodes to targets and rejects unknown identities", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a"})
		Expect(mapping.ID(&corev3.Node{Id: "node-a"})).To(Equal("target-a"))
		Expect(mapping.ID(&corev3.Node{Id: "unknown"})).To(BeEmpty())

		recorder := &observationRecorder{}
		callbacks := newCallbacks(mapping, recorder, newConnections())
		request := &discoveryv3.DiscoveryRequest{Node: &corev3.Node{Id: "unknown"}}
		Expect(callbacks.onStreamRequest(1, request)).To(MatchError(ContainSubstring("not mapped")))
	})

	It("correlates and sanitizes ACK and NACK observations", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a"})
		recorder := &observationRecorder{}
		callbacks := newCallbacks(mapping, recorder, newConnections())
		initial := &discoveryv3.DiscoveryRequest{Node: &corev3.Node{Id: "node-a"}}
		Expect(callbacks.onStreamRequest(7, initial)).To(Succeed())
		response := &discoveryv3.DiscoveryResponse{
			TypeUrl: resource.ListenerType, VersionInfo: "3", Nonce: "nonce-a",
		}
		callbacks.onStreamResponse(context.Background(), 7, initial, response)
		nack := &discoveryv3.DiscoveryRequest{
			TypeUrl: resource.ListenerType, ResponseNonce: "nonce-a",
			ErrorDetail: status.New(codes.InvalidArgument, "bad\nsecret").Proto(),
		}
		Expect(callbacks.onStreamRequest(7, nack)).To(Succeed())
		Expect(recorder.observations).To(HaveLen(1))
		Expect(recorder.observations[0].State).To(Equal(xdsapi.DeliveryState_DELIVERY_STATE_NACK))
		Expect(recorder.observations[0].Generation).To(Equal(uint64(3)))
		Expect(recorder.observations[0].ErrorDetail).To(Equal("bad secret"))
	})

	It("truncates NACK details without splitting UTF-8", func() {
		detail := strings.Repeat("a", maxErrorDetailLength-1) + "€"

		sanitized := sanitizeError(detail)
		Expect(utf8.ValidString(sanitized)).To(BeTrue())
		Expect(sanitized).To(Equal(strings.Repeat("a", maxErrorDetailLength-1)))
	})
})

var _ = Describe("Durable service", func() {
	var (
		ctx   context.Context
		store *storage.Store
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		store, err = storage.Open(ctx, filepath.Join(GinkgoT().TempDir(), "xds.db"), 10)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("publishes idempotently without waiting for nodes", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a"})
		cache := cachev3.NewSnapshotCache(true, mapping, nil)
		service, err := New(ctx, store, cache, mapping)
		Expect(err).NotTo(HaveOccurred())
		bundle := validBundle()

		first, err := service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: bundle})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.PersistedGeneration).To(Equal(uint64(1)))
		Expect(first.Idempotent).To(BeFalse())
		second, err := service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: bundle})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Idempotent).To(BeTrue())

		result, err := service.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Converged).To(BeTrue())
		Expect(result.ConnectedNodeIds).To(BeEmpty())
	})

	It("does not reactivate a historical digest replay", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a"})
		cache := cachev3.NewSnapshotCache(true, mapping, nil)
		service, err := New(ctx, store, cache, mapping)
		Expect(err).NotTo(HaveOccurred())
		older := validBundle()
		_, err = service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: older})
		Expect(err).NotTo(HaveOccurred())

		newer := proto.Clone(older).(*xdsapi.Bundle)
		newer.PublisherGeneration = "source-2"
		newer.CompilerVersion = "compiler-b"
		Expect(xdsapi.SetDigest(newer)).To(Succeed())
		_, err = service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: newer})
		Expect(err).NotTo(HaveOccurred())

		replay, err := service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: older})
		Expect(err).NotTo(HaveOccurred())
		Expect(replay.Idempotent).To(BeTrue())
		Expect(replay.Activated).To(BeFalse())
		statusResult, err := service.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(statusResult.PersistedGeneration).To(Equal(uint64(2)))
		Expect(statusResult.Digest).To(Equal(newer.Digest))
	})

	It("keeps durable activation on cache failure and repairs it on startup", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a"})
		baseCache := cachev3.NewSnapshotCache(true, mapping, nil)
		cache := &failingCache{SnapshotCache: baseCache, fail: true}
		service, err := New(ctx, store, cache, mapping)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: validBundle()})
		Expect(status.Code(err)).To(Equal(codes.Internal))
		active, exists, err := store.Active(ctx, "target-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(exists).To(BeTrue())
		Expect(active.Generation).To(Equal(uint64(1)))
		result, err := service.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Activated).To(BeFalse())

		restoredCache := cachev3.NewSnapshotCache(true, mapping, nil)
		restored, err := New(ctx, store, restoredCache, mapping)
		Expect(err).NotTo(HaveOccurred())
		snapshot, err := restoredCache.GetSnapshot("target-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshot.GetVersion(resource.ListenerType)).To(Equal("1"))
		result, err = restored.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Activated).To(BeTrue())
	})

	It("requires ACKs for all four resource types from every connected node", func() {
		mapping := NewNodeMapping(map[string]string{"node-a": "target-a", "node-b": "target-a"})
		cache := cachev3.NewSnapshotCache(true, mapping, nil)
		service, err := New(ctx, store, cache, mapping)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.PublishBundle(ctx, &xdsapi.PublishBundleRequest{Bundle: validBundle()})
		Expect(err).NotTo(HaveOccurred())
		service.connections.connect(1, "node-a")
		service.connections.connect(2, "node-b")
		for _, nodeID := range []string{"node-a", "node-b"} {
			for _, typeURL := range supportedTypeURLs {
				Expect(store.RecordObservation(ctx, "target-a", &xdsapi.DeliveryObservation{
					NodeId: nodeID, TypeUrl: typeURL, Generation: 1,
					State:      xdsapi.DeliveryState_DELIVERY_STATE_ACK,
					ObservedAt: timestamppb.Now(), Nonce: strconv.Itoa(len(typeURL)),
				})).To(Succeed())
			}
		}
		result, err := service.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Converged).To(BeTrue())
		Expect(result.ConnectedNodeIds).To(ConsistOf("node-a", "node-b"))

		Expect(store.RecordObservation(ctx, "target-a", &xdsapi.DeliveryObservation{
			NodeId: "node-b", TypeUrl: supportedTypeURLs[0], Generation: 1,
			State: xdsapi.DeliveryState_DELIVERY_STATE_NACK, ObservedAt: timestamppb.Now(),
		})).To(Succeed())
		result, err = service.GetStatus(ctx, &xdsapi.GetStatusRequest{TargetId: "target-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Converged).To(BeFalse())
	})
})
