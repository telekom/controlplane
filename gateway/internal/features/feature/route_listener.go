// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ features.Feature = &RouteListenerFeature{}

var InstanceRouteListenerFeature = &RouteListenerFeature{
	priority: InstanceLastMileSecurityFeature.Priority() + 2,
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

	for _, rl := range builder.GetRouteListeners() {
		jc.RouteListener[plugin.ConsumerId(rl.Spec.Consumer)] = plugin.RouteListenerEntry{
			Issue:        rl.Spec.Issue,
			ServiceOwner: rl.Spec.ServiceOwner,
		}
	}

	return nil
}
