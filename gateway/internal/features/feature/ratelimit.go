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

var _ features.Feature = &RateLimitFeature{}

// RateLimitFeature takes precedence over CustomScopesFeature
type RateLimitFeature struct {
	priority int
}

var InstanceRateLimitFeature = &RateLimitFeature{
	priority: 10,
}

func (f *RateLimitFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeRateLimit
}

func (f *RateLimitFeature) Priority() int {
	return f.priority
}

func (f *RateLimitFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	hasRateLimitConfigured := false

	return !route.Spec.PassThrough && hasRateLimitConfigured
}

func (f *RateLimitFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rateLimitPlugin := builder.RateLimitPlugin()

	// TODO: get this from gateway or something
	rateLimitPlugin.Config.Policy = plugin.PolicyRedis
	rateLimitPlugin.Config.RedisConfig = plugin.RedisConfig{
		Host: "redis",
		Port: 443,
	}

	rateLimitPlugin.Config.Limits = plugin.Limits{
		Consumer: &plugin.LimitConfig{
			Second: 10,
			Minute: 100,
			Hour:   1000,
		},
		Service: &plugin.LimitConfig{
			Second: 20,
			Minute: 200,
			Hour:   2000,
		},
	}

	return nil
}
