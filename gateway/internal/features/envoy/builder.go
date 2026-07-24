// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/go-logr/logr"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.EnvoyFeatureBuilder = &Builder{}

type Builder struct {
	client XdsClient

	AllowedConsumers []*gatewayv1.ConsumeRoute

	Route    *gatewayv1.Route
	Consumer *gatewayv1.Consumer
	Gateway  *gatewayv1.Gateway

	Features map[gatewayv1.FeatureType]features.EnvoyFeature

	// httpFilters are the extra HTTP filters contributed by features during
	// Apply, in application order. buildListener inserts them before the router.
	httpFilters []*hcmv3.HttpFilter
}

var NewEnvoyFeatureBuilder = func(xdsClient XdsClient, route *gatewayv1.Route, consumer *gatewayv1.Consumer, gateway *gatewayv1.Gateway) features.EnvoyFeatureBuilder {
	return &Builder{
		client:           xdsClient,
		AllowedConsumers: []*gatewayv1.ConsumeRoute{},
		Route:            route,
		Consumer:         consumer,
		Gateway:          gateway,
		Features:         map[gatewayv1.FeatureType]features.EnvoyFeature{},
	}
}

// EnableFeature implements [features.EnvoyFeatureBuilder].
func (b *Builder) EnableFeature(f features.EnvoyFeature) {
	b.Features[f.Name()] = f
}

// AddHTTPFilter implements [features.EnvoyFeatureBuilder].
func (b *Builder) AddHTTPFilter(name string, typedConfig *anypb.Any) {
	b.httpFilters = append(b.httpFilters, &hcmv3.HttpFilter{
		Name:       name,
		ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: typedConfig},
	})
}

// GetRoute implements [features.FeatureBuilder].
func (b *Builder) GetRoute() (*gatewayv1.Route, bool) {
	if b.Route == nil {
		return nil, false
	}
	return b.Route, true
}

// GetConsumer implements [features.FeatureBuilder].
func (b *Builder) GetConsumer() (*gatewayv1.Consumer, bool) {
	if b.Consumer == nil {
		return nil, false
	}
	return b.Consumer, true
}

// GetGateway implements [features.FeatureBuilder].
func (b *Builder) GetGateway() *gatewayv1.Gateway {
	return b.Gateway
}

// GetAllowedConsumers implements [features.FeatureBuilder].
func (b *Builder) GetAllowedConsumers() []*gatewayv1.ConsumeRoute {
	return b.AllowedConsumers
}

// AddAllowedConsumers implements [features.FeatureBuilder].
func (b *Builder) AddAllowedConsumers(consumers ...*gatewayv1.ConsumeRoute) {
	b.AllowedConsumers = append(b.AllowedConsumers, consumers...)
}

// Build implements [features.FeatureBuilder]. It applies the enabled features in
// priority order (each may contribute HTTP filters via AddHTTPFilter), then
// renders the core-routing xDS resources for the Route and publishes them as one
// consistent node snapshot.
func (b *Builder) Build(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.features.builder")
	if b.Route == nil {
		return features.ErrNoRoute
	}
	log = log.WithValues("route", b.Route.Name)

	for _, f := range features.SortFeatures(features.ToSlice(b.Features)) {
		if f.IsUsed(ctx, b) {
			log.V(1).Info("Applying feature", "name", f.Name())
			if err := f.Apply(ctx, b); err != nil {
				return err
			}
		} else {
			log.V(1).Info("Feature is not used", "name", f.Name())
		}
	}

	upstreams := b.Route.Spec.Backend.Upstreams
	if len(upstreams) == 0 {
		return ctrlerrors.BlockedErrorf("route %q has no upstream", b.Route.Name)
	}
	// ponytail: single-upstream only; weighted_clusters for len>1 is a later
	// increment (see RT-04/RT-11). Take the first target.
	upstream := upstreams[0]

	bundle, err := renderCoreRouting(b.Route, upstream, b.httpFilters)
	if err != nil {
		return ctrlerrors.RetryableErrorf("rendering xDS for route %q: %v", b.Route.Name, err)
	}

	nodeID := nodeIDForRoute(b.Route)
	log.V(0).Info("Publishing route snapshot", "nodeID", nodeID)
	if err := b.client.SetSnapshotFor(ctx, nodeID, bundle); err != nil {
		return ctrlerrors.RetryableErrorf("publishing snapshot for route %q: %v", b.Route.Name, err)
	}
	return nil
}

// BuildForConsumer implements [features.FeatureBuilder].
// ponytail: consumer-scoped features (e.g. IpRestriction) are a later increment.
func (b *Builder) BuildForConsumer(context.Context) error {
	return ctrlerrors.BlockedErrorf("Envoy BuildForConsumer is not implemented yet")
}

// nodeIDForRoute keys the snapshot on the Gateway the Route targets, matching the
// nodeHash convention (node.metadata.role == Gateway identity).
//
// ponytail: keyed per-Route on the Gateway name, but each Route currently
// overwrites the Gateway's whole snapshot (single-route-per-node). Accumulate all
// routes of a Gateway into one bundle before this is production-shaped.
func nodeIDForRoute(route *gatewayv1.Route) string {
	return route.Spec.GatewayRef.Name
}
