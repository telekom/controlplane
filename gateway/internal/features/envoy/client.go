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
	"strconv"
	"sync/atomic"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/go-logr/logr"
)

// PocNodeID is the single hardcoded xDS node id used by the POC.
// ponytail: single node + whole-snapshot overwrite → one route at a time.
// Upgrade path: per-node IDs that accumulate a node's routes.
const PocNodeID = "poc-gateway-node"

// ResourceBundle is the set of xDS resources emitted for a single Build.
// It mirrors what the Kong path writes as a route + its plugins, but shaped
// as go-control-plane resources.
type ResourceBundle struct {
	Listeners []*listenerv3.Listener
	Clusters  []*clusterv3.Cluster
	Routes    []*routev3.RouteConfiguration
}

// XdsClient is the write seam, analog of client.KongClient. It publishes a
// bundle as an internally-consistent snapshot for a given node.
type XdsClient interface {
	SetSnapshotFor(ctx context.Context, nodeID string, bundle ResourceBundle) error
}

var _ XdsClient = &snapshotClient{}

type snapshotClient struct {
	cache   cachev3.SnapshotCache
	version atomic.Uint64
}

// NewXdsClient wraps a go-control-plane SnapshotCache as an XdsClient.
func NewXdsClient(cache cachev3.SnapshotCache) XdsClient {
	return &snapshotClient{cache: cache}
}

func (c *snapshotClient) SetSnapshotFor(ctx context.Context, nodeID string, bundle ResourceBundle) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.xds").WithValues("node", nodeID)

	version := strconv.FormatUint(c.version.Add(1), 10)

	resources := map[resource.Type][]cachetypes.Resource{
		resource.ClusterType:  toResources(bundle.Clusters),
		resource.ListenerType: toResources(bundle.Listeners),
	}
	// RouteConfigurations are only published as standalone RDS resources when
	// the listener references them by name. The POC inlines the RouteConfig in
	// the HCM, so they must NOT appear in the snapshot map (else Consistent()
	// reports a reference/resource length mismatch).
	if len(bundle.Routes) > 0 {
		resources[resource.RouteType] = toResources(bundle.Routes)
	}

	snap, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}
	if err := snap.Consistent(); err != nil {
		return fmt.Errorf("snapshot is not consistent: %w", err)
	}
	if err := c.cache.SetSnapshot(ctx, nodeID, snap); err != nil {
		return fmt.Errorf("setting snapshot: %w", err)
	}

	log.V(0).Info("Published xDS snapshot",
		"version", version,
		"clusters", len(bundle.Clusters),
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
