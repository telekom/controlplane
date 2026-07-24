// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package envoy contains the go-control-plane-based counterpart to the Kong
// FeaturesBuilder. It translates the same Route/Consumer source fields into
// Envoy xDS resources and publishes them via a SnapshotCache.
package envoy

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	cachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/proto"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// PocNodeID remains as a zero-value compatibility symbol for the out-of-scope
// xdsdemo command. Snapshot publication no longer uses a global node ID.
// Deprecated: use NodeIDForGateway.
var PocNodeID string

// NodeIDForGateway returns the stable xDS node identity for a Gateway.
func NodeIDForGateway(gateway *gatewayv1.Gateway) (string, error) {
	if gateway == nil || gateway.UID == "" {
		return "", fmt.Errorf("gateway UID is empty")
	}
	return "gateway:" + string(gateway.UID), nil
}

// ResourceBundle is the set of xDS resources emitted for a single Build.
// It mirrors what the Kong path writes as a route + its plugins, but shaped
// as go-control-plane resources.
type ResourceBundle struct {
	Listeners []*listenerv3.Listener
	Clusters  []*clusterv3.Cluster
	Routes    []*routev3.RouteConfiguration
}

// XdsClient coordinates route contributions into complete Gateway snapshots.
type XdsClient interface {
	PublishRoute(ctx context.Context, gateway *gatewayv1.Gateway, routeID string, expectedRouteIDs []string, bundle ResourceBundle) error
	DeleteRoute(ctx context.Context, gateway *gatewayv1.Gateway, routeID string) error
	DeleteRouteWithExpected(ctx context.Context, gateway *gatewayv1.Gateway, routeID string, expectedRouteIDs []string) error
	ClearGateway(ctx context.Context, gateway *gatewayv1.Gateway) error
}

type snapshotClient struct {
	cache cachev3.SnapshotCache
}

// NewXdsClient wraps a go-control-plane SnapshotCache as an XdsClient.
func NewXdsClient(cache cachev3.SnapshotCache) XdsClient {
	return NewGatewaySnapshotCoordinator(&snapshotClient{cache: cache})
}

func (c *snapshotClient) setSnapshotFor(ctx context.Context, nodeID string, bundle ResourceBundle) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.xds").WithValues("node", nodeID)

	version, err := generationFor(bundle)
	if err != nil {
		return fmt.Errorf("calculating snapshot generation: %w", err)
	}

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

func (c *snapshotClient) clearSnapshotFor(nodeID string) {
	c.cache.ClearSnapshot(nodeID)
}

// generationFor derives one stable version for every resource type in a
// snapshot. Sorting by type and resource name makes it independent of render
// and reconciliation order.
func generationFor(bundle ResourceBundle) (string, error) {
	type namedResource struct {
		typeURL string
		name    string
		value   proto.Message
	}
	resources := make([]namedResource, 0, len(bundle.Clusters)+len(bundle.Listeners)+len(bundle.Routes))
	for _, r := range bundle.Clusters {
		resources = append(resources, namedResource{resource.ClusterType, r.GetName(), r})
	}
	for _, r := range bundle.Listeners {
		resources = append(resources, namedResource{resource.ListenerType, r.GetName(), r})
	}
	for _, r := range bundle.Routes {
		resources = append(resources, namedResource{resource.RouteType, r.GetName(), r})
	}
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].typeURL != resources[j].typeURL {
			return resources[i].typeURL < resources[j].typeURL
		}
		return resources[i].name < resources[j].name
	})

	hash := sha256.New()
	marshal := proto.MarshalOptions{Deterministic: true}
	for _, r := range resources {
		data, err := marshal.Marshal(r.value)
		if err != nil {
			return "", fmt.Errorf("marshalling %s %q: %w", r.typeURL, r.name, err)
		}
		_, _ = hash.Write([]byte(r.typeURL))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(r.name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func toResources[T cachetypes.Resource](in []T) []cachetypes.Resource {
	out := make([]cachetypes.Resource, 0, len(in))
	for _, r := range in {
		out = append(out, r)
	}
	return out
}
