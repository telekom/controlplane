// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/proto"
)

// ResourceBundle is the full set of xDS resources for a single node (Gateway).
// It is a snapshot: all resources are pushed together so that RDS/CDS/EDS stay
// mutually consistent.
type ResourceBundle struct {
	Listeners []*listenerv3.Listener
	Clusters  []*clusterv3.Cluster
	Routes    []*routev3.RouteConfiguration
	Endpoints []*endpointv3.ClusterLoadAssignment
}

// XdsClient publishes an xDS snapshot for a node into the underlying cache.
type XdsClient interface {
	// SetSnapshotFor publishes the bundle for the given node. It is a no-op if
	// the bundle content is identical to the last published snapshot for that
	// node (hash-diff gate), so unrelated reconciles do not force connected
	// Envoy proxies to re-ACK an unchanged snapshot.
	SetSnapshotFor(ctx context.Context, nodeID string, bundle ResourceBundle) error
}

var _ XdsClient = (*XdsCache)(nil)

// XdsCache is the shared, long-lived xDS snapshot cache. One instance is created
// at process start; the ADS [Server] serves from it and reconcilers publish into
// it. It must be shared, not created per reconcile.
type XdsCache struct {
	cache cachev3.SnapshotCache

	mu           sync.Mutex
	lastVersions map[string]string
}

// NewXdsCache creates the shared snapshot cache. ADS mode is enabled so Envoy
// receives resources in dependency order (CDS before EDS, LDS before RDS).
func NewXdsCache(logger logr.Logger) *XdsCache {
	return &XdsCache{
		cache:        cachev3.NewSnapshotCache(true, nodeHash{}, newCacheLogger(logger)),
		lastVersions: map[string]string{},
	}
}

// Cache exposes the underlying cache so the ADS [Server] can serve from the
// same instance that reconcilers publish into.
func (c *XdsCache) Cache() cachev3.Cache {
	return c.cache
}

// SetSnapshotFor implements [XdsClient].
func (c *XdsCache) SetSnapshotFor(ctx context.Context, nodeID string, bundle ResourceBundle) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.xds-cache").WithValues("nodeID", nodeID)

	resources := bundle.toResourceMap()
	version := hashResources(resources)

	c.mu.Lock()
	unchanged := c.lastVersions[nodeID] == version
	c.mu.Unlock()

	if unchanged {
		log.V(1).Info("Snapshot unchanged, skipping push", "version", version)
		return nil
	}

	snapshot, err := cachev3.NewSnapshot(version, resources)
	if err != nil {
		return fmt.Errorf("creating snapshot for node %q: %w", nodeID, err)
	}

	if err := c.cache.SetSnapshot(ctx, nodeID, snapshot); err != nil {
		return fmt.Errorf("setting snapshot for node %q: %w", nodeID, err)
	}

	c.mu.Lock()
	c.lastVersions[nodeID] = version
	c.mu.Unlock()

	log.Info("Published snapshot", "version", version,
		"listeners", len(bundle.Listeners), "clusters", len(bundle.Clusters),
		"routes", len(bundle.Routes), "endpoints", len(bundle.Endpoints))
	return nil
}

// toResourceMap converts the bundle into the type-keyed map the cache expects.
func (b ResourceBundle) toResourceMap() map[resourcev3.Type][]cachetypes.Resource {
	m := map[resourcev3.Type][]cachetypes.Resource{}
	if len(b.Listeners) > 0 {
		m[resourcev3.ListenerType] = toResources(b.Listeners)
	}
	if len(b.Clusters) > 0 {
		m[resourcev3.ClusterType] = toResources(b.Clusters)
	}
	if len(b.Routes) > 0 {
		m[resourcev3.RouteType] = toResources(b.Routes)
	}
	if len(b.Endpoints) > 0 {
		m[resourcev3.EndpointType] = toResources(b.Endpoints)
	}
	return m
}

func toResources[T cachetypes.Resource](in []T) []cachetypes.Resource {
	out := make([]cachetypes.Resource, len(in))
	for i, r := range in {
		out[i] = r
	}
	return out
}

// hashResources computes a deterministic content hash of all resources. The
// same content always yields the same version, which gives restart and
// cross-replica determinism (Envoy sees no change if content is identical).
//
// Each resource is marshaled deterministically and hashed; the per-resource
// hashes are combined by XOR so the result is independent of ordering within a
// type. The resulting version is stable across process restarts.
func hashResources(resources map[resourcev3.Type][]cachetypes.Resource) string {
	// Sort type URLs for a stable walk order.
	typeURLs := make([]string, 0, len(resources))
	for t := range resources {
		typeURLs = append(typeURLs, t)
	}
	sort.Strings(typeURLs)

	marshal := proto.MarshalOptions{Deterministic: true}
	var combined uint64
	for _, t := range typeURLs {
		for _, r := range resources[t] {
			h := fnv.New64a()
			_, _ = h.Write([]byte(t))
			if msg, ok := r.(proto.Message); ok {
				b, err := marshal.Marshal(msg)
				if err == nil {
					_, _ = h.Write(b)
				}
			}
			combined ^= h.Sum64()
		}
	}
	return fmt.Sprintf("%016x", combined)
}
