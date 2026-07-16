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
	notPassThrough := !route.Spec.PassThrough
	isPrimaryRoute := !route.IsProxy()
	isFailoverSecondary := route.Spec.Type == gatewayv1.RouteTypeSecondary

	// Claims only apply to the platform-managed LMS token, not an external IDP's
	// token or a basic-auth upstream.
	if route.Spec.Security.M2M != nil &&
		(route.Spec.Security.M2M.ExternalIDP != nil || route.Spec.Security.M2M.Basic != nil) {
		return false
	}

	return notPassThrough && (isPrimaryRoute || isFailoverSecondary)
}

func (f *ClaimsFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	jumperConfig := builder.JumperConfig()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	// Provider exposure claims -> default bucket (applies to all consumers)
	if route.Spec.Security.M2M != nil && len(route.Spec.Security.M2M.Claims) > 0 {
		jumperConfig.Claims[plugin.ConsumerId(DefaultProviderKey)] = toPluginClaims(route.Spec.Security.M2M.Claims)
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
