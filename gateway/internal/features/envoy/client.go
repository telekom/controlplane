// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package envoy contains the go-control-plane-based counterpart to the Kong
// FeaturesBuilder. It translates the same Route/Consumer source fields into
// Envoy xDS resources and publishes them via a SnapshotCache.
package envoy

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PocNodeID is the single hardcoded xDS node id used by the POC.
// ponytail: single node + whole-snapshot overwrite → one route at a time.
// Upgrade path: per-node IDs that accumulate a node's routes.
const PocNodeID = "poc-gateway-node"

// TargetIdentity identifies one Gateway configuration target.
type TargetIdentity struct {
	Environment string
	Namespace   string
	Name        string
	UID         types.UID
}

// SourceReference identifies one Kubernetes input revision used to compile a bundle.
type SourceReference struct {
	Kind       string
	Namespace  string
	Name       string
	UID        types.UID
	Generation int64
}

// SourceMetadata describes the complete source aggregate used for a bundle.
type SourceMetadata struct {
	Resources []SourceReference
}

// ResourceBundle is a complete, deterministic LDS/RDS/CDS/EDS resource set.
type ResourceBundle struct {
	Target    TargetIdentity
	Source    SourceMetadata
	Listeners []*listenerv3.Listener
	Clusters  []*clusterv3.Cluster
	Routes    []*routev3.RouteConfiguration
	Endpoints []*endpointv3.ClusterLoadAssignment
}

// Sort orders bundle metadata and xDS resources by stable identity.
func (b *ResourceBundle) Sort() {
	sort.Slice(b.Source.Resources, func(i, j int) bool {
		a, c := b.Source.Resources[i], b.Source.Resources[j]
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		if a.Namespace != c.Namespace {
			return a.Namespace < c.Namespace
		}
		return a.Name < c.Name
	})
	sort.Slice(b.Listeners, func(i, j int) bool { return b.Listeners[i].GetName() < b.Listeners[j].GetName() })
	sort.Slice(b.Routes, func(i, j int) bool { return b.Routes[i].GetName() < b.Routes[j].GetName() })
	sort.Slice(b.Clusters, func(i, j int) bool { return b.Clusters[i].GetName() < b.Clusters[j].GetName() })
	sort.Slice(b.Endpoints, func(i, j int) bool {
		return b.Endpoints[i].GetClusterName() < b.Endpoints[j].GetClusterName()
	})
}

func sourceReference(kind string, obj metav1.Object) SourceReference {
	return SourceReference{
		Kind: kind, Namespace: obj.GetNamespace(), Name: obj.GetName(), UID: obj.GetUID(), Generation: obj.GetGeneration(),
	}
}

// XdsClient is the write seam, analog of client.KongClient. It publishes a
// bundle as an internally-consistent snapshot for a given node.
type XdsClient interface {
	Publish(ctx context.Context, target TargetIdentity, source SourceMetadata, bundle *ResourceBundle) error
}

var _ XdsClient = &snapshotClient{}

type snapshotClient struct {
	cache   cachev3.SnapshotCache
	nodeID  string
	version atomic.Uint64
}

// NewXdsClient wraps a go-control-plane SnapshotCache as an XdsClient.
func NewXdsClient(cache cachev3.SnapshotCache, nodeIDs ...string) XdsClient {
	nodeID := PocNodeID
	if len(nodeIDs) > 0 {
		nodeID = nodeIDs[0]
	}
	return &snapshotClient{cache: cache, nodeID: nodeID}
}

func (c *snapshotClient) Publish(
	ctx context.Context,
	target TargetIdentity,
	source SourceMetadata,
	bundle *ResourceBundle,
) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.xds").WithValues("node", c.nodeID, "target", target.Name)
	bundle.Target = target
	bundle.Source = source

	version := strconv.FormatUint(c.version.Add(1), 10)

	resources := map[resource.Type][]cachetypes.Resource{
		resource.ClusterType:  toResources(bundle.Clusters),
		resource.EndpointType: toResources(bundle.Endpoints),
		resource.ListenerType: toResources(bundle.Listeners),
		resource.RouteType:    toResources(bundle.Routes),
	}

	snap, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}
	if err := snap.Consistent(); err != nil {
		return fmt.Errorf("snapshot is not consistent: %w", err)
	}
	if err := c.cache.SetSnapshot(ctx, c.nodeID, snap); err != nil {
		return fmt.Errorf("setting snapshot: %w", err)
	}

	log.V(0).Info("Published xDS snapshot",
		"version", version,
		"clusters", len(bundle.Clusters),
		"endpoints", len(bundle.Endpoints),
		"routes", len(bundle.Routes),
		"listeners", len(bundle.Listeners))
	return nil
}

func toResources[T cachetypes.Resource](in []T) []cachetypes.Resource {
	out := make([]cachetypes.Resource, 0, len(in))
	for _, r := range in {
		out = append(out, r)
	}
	return out
}
