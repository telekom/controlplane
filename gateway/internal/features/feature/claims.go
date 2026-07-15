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

var _ features.Feature = &ClaimsFeature{}

// ClaimsFeature writes token claims into JumperConfig.Claims. Provider exposure
// claims land in the "default" bucket (all consumers); per-consumer overrides land
// under the consumer's client-id key. Modeled on CustomScopesFeature.
type ClaimsFeature struct {
	priority int
}

var InstanceClaimsFeature = &ClaimsFeature{
	priority: 10,
}

func (f *ClaimsFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeClaims
}

func (f *ClaimsFeature) Priority() int {
	return f.priority
}

func (f *ClaimsFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	notPassThrough := !route.Spec.PassThrough
	isPrimaryRoute := !route.IsProxy()
	isFailoverSecondary := route.Spec.Type == gatewayv1.RouteTypeSecondary

	return notPassThrough && (isPrimaryRoute || isFailoverSecondary)
}

func (f *ClaimsFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	jumperConfig := builder.JumperConfig()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	if len(jumperConfig.OAuth) > 0 {
		// External IDP owns the token; claims only apply to the platform-managed LMS token.
		return nil
	}

	// Provider exposure claims -> default bucket (applies to all consumers)
	if route.Spec.Security.M2M != nil && len(route.Spec.Security.M2M.Claims) > 0 {
		jumperConfig.Claims[plugin.ConsumerId(DefaultProviderKey)] = toPluginClaims(route.Spec.Security.M2M.Claims)
	}

	// Per-consumer overrides -> consumer's client-id key
	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.Spec.Security != nil && consumer.Spec.Security.M2M != nil && len(consumer.Spec.Security.M2M.Claims) > 0 {
			jumperConfig.Claims[plugin.ConsumerId(consumer.Spec.ConsumerName)] = toPluginClaims(consumer.Spec.Security.M2M.Claims)
		}
	}

	return nil
}

func toPluginClaims(claims []gatewayv1.Claim) []plugin.Claim {
	out := make([]plugin.Claim, 0, len(claims))
	for _, c := range claims {
		out = append(out, plugin.Claim{
			Key:       c.Key,
			Value:     c.Value,
			ValueFrom: string(c.ValueFrom),
		})
	}
	return out
}
