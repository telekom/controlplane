// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &RouteListenerFeature{}

var InstanceRouteListenerFeature = &RouteListenerFeature{
	priority: InstanceLastMileSecurityFeature.Priority() + 3,
}

type RouteListenerFeature struct {
	priority int
}

// Name implements features.Feature.
func (f *RouteListenerFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeRouteListener
}

// Priority implements features.Feature.
func (f *RouteListenerFeature) Priority() int {
	return f.priority
}

// IsUsed implements features.Feature.
func (f *RouteListenerFeature) IsUsed(_ context.Context, builder features.FeaturesBuilder) bool {
	return len(builder.GetRouteListeners()) > 0
}

// Apply implements features.Feature.
func (f *RouteListenerFeature) Apply(_ context.Context, builder features.FeaturesBuilder) error {
	jc := builder.JumperConfig()
	if jc.RouteListener == nil {
		jc.RouteListener = make(map[plugin.ConsumerId]plugin.RouteListenerEntry)
	}

	// jumper_config.routeListener is keyed by consumer only, so two RouteListeners
	// for the same consumer on this route cannot both be represented. Sort by name so
	// the outcome does not depend on the order the RouteListeners were listed.
	routeListeners := slices.Clone(builder.GetRouteListeners())
	slices.SortStableFunc(routeListeners, func(a, b *gatewayv1.RouteListener) int {
		return cmp.Compare(a.Name, b.Name)
	})

	for _, rl := range routeListeners {
		cid := plugin.ConsumerId(rl.Spec.Consumer)
		if existing, ok := jc.RouteListener[cid]; ok {
			return errors.Errorf("conflicting RouteListeners for consumer %q: issue %q vs %q (jumper supports one listener entry per consumer per route)",
				cid, existing.Issue, rl.Spec.Issue)
		}

		jc.RouteListener[cid] = plugin.RouteListenerEntry{
			Issue:        rl.Spec.Issue,
			ServiceOwner: rl.Spec.ServiceOwner,
		}

		// jumper reads the publisher credentials from a single top-level
		// gatewayClient. Every listener on a route resolves the same zone-level
		// "gateway" Client, so writing it once per listener is idempotent — this
		// mirrors the legacy gateway
		// (KongCeClient.appendOrUpdateListenerForRequestTransformerPlugin).
		//
		// Secret is intentionally left unset: it must be resolved through
		// secret-manager (secrets.Get) rather than carried literally in a CR spec,
		// and RouteListenerSpec.GatewayClient does not hold a secret reference yet.
		jc.GatewayClient = &plugin.GatewayClient{
			Id:     rl.Spec.GatewayClient.ClientId,
			Issuer: rl.Spec.GatewayClient.Issuer,
		}
	}

	builder.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostListenerUrl))

	return nil
}
