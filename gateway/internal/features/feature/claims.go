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

// ClaimsFeature writes provider exposure token claims into JumperConfig.Claims.
// Claims land in the "default" bucket (applies to all consumers). Modeled on
// CustomScopesFeature.
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
	if route.Spec.PassThrough {
		return false
	}

	// The claims are only send to the provider upstream
	// Therefore, this feature is only relevant for Routes that are primary or failover secondary.
	// For other routes, the claims are not relevant.
	isPrimary := route.IsPrimary()
	isFailoverSecondary := route.IsFailoverSecondary()

	if !isPrimary && !isFailoverSecondary {
		return false
	}

	// Check if the route has claims configured in its security configuration
	isConfigured := false
	if route.Spec.Security.HasM2MClaims() {
		isConfigured = true
	}
	if isFailoverSecondary && HasFailoverSecurity(route) && route.Spec.Traffic.Failover.Security.HasM2MClaims() {
		isConfigured = true
	}

	return isConfigured
}

func (f *ClaimsFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	jumperConfig := builder.JumperConfig()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	security := route.Spec.Security
	if route.IsFailoverSecondary() && HasFailoverSecurity(route) {
		security = route.Spec.Traffic.Failover.Security
	}

	// Provider exposure claims -> default bucket (applies to all consumers)
	if security.M2M != nil && len(security.M2M.Claims) > 0 {
		jumperConfig.Claims[plugin.ConsumerId(DefaultProviderKey)] = toPluginClaims(security.M2M.Claims)
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
