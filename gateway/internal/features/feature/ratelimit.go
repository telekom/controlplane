// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	"github.com/pkg/errors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
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
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}

	anyConsumerHasRateLimiting := false
	consumers := builder.GetAllowedConsumers()
	for _, allowedConsumer := range consumers {
		if allowedConsumer.HasTrafficRateLimit() {
			anyConsumerHasRateLimiting = true
			break
		}
	}

	return !route.Spec.PassThrough && (route.HasRateLimit() || anyConsumerHasRateLimiting)
}

func (f *RateLimitFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return nil
	}

	var rateLimitPlugin *plugin.RateLimitPlugin
	if !route.IsProxy() {
		if route.HasRateLimit() {
			rateLimitPlugin = builder.RateLimitPluginRoute()
			if err = setCommonConfigs(rateLimitPlugin, builder.GetGateway()); err != nil {
				return err
			}
			rateLimitPlugin.Config.Limits = plugin.Limits{
				Service: &plugin.LimitConfig{
					Second: route.Spec.Traffic.RateLimit.Limits.Second,
					Minute: route.Spec.Traffic.RateLimit.Limits.Minute,
					Hour:   route.Spec.Traffic.RateLimit.Limits.Hour,
				},
			}
			rateLimitPlugin = setOptions(rateLimitPlugin, route.Spec.Traffic.RateLimit.Options)
		}
	}

	for _, allowedConsumer := range builder.GetAllowedConsumers() {
		routeRef := allowedConsumer.Spec.Route
		if !routeRef.Equals(route) {
			continue
		}
		if allowedConsumer.HasTrafficRateLimit() || route.HasRateLimit() {
			rateLimitPlugin = builder.RateLimitPluginConsumeRoute(allowedConsumer)
			if err = setCommonConfigs(rateLimitPlugin, builder.GetGateway()); err != nil {
				return err
			}
		}
		if allowedConsumer.HasTrafficRateLimit() {
			rateLimitPlugin.Config.Limits.Consumer = &plugin.LimitConfig{
				Second: allowedConsumer.Spec.Traffic.RateLimit.Limits.Second,
				Minute: allowedConsumer.Spec.Traffic.RateLimit.Limits.Minute,
				Hour:   allowedConsumer.Spec.Traffic.RateLimit.Limits.Hour,
			}
		}
		if route.HasRateLimit() {
			rateLimitPlugin.Config.Limits.Service = &plugin.LimitConfig{
				Second: route.Spec.Traffic.RateLimit.Limits.Second,
				Minute: route.Spec.Traffic.RateLimit.Limits.Minute,
				Hour:   route.Spec.Traffic.RateLimit.Limits.Hour,
			}
			rateLimitPlugin = setOptions(rateLimitPlugin, route.Spec.Traffic.RateLimit.Options)
		}
	}

	return nil
}

func setCommonConfigs(rateLimitPlugin *plugin.RateLimitPlugin, gateway *gatewayv1.Gateway) error {
	rateLimitPlugin.Config.Policy = plugin.PolicyRedis
	redisPassword, err := secretManagerApi.Get(context.Background(), gateway.Spec.Redis.Password)
	if err != nil {
		return errors.Wrapf(err, "cannot get redis password for gateway %s", gateway.GetName())
	}
	rateLimitPlugin.Config.RedisConfig = plugin.RedisConfig{
		Host:     gateway.Spec.Redis.Host,
		Port:     gateway.Spec.Redis.Port,
		Password: redisPassword,
	}
	rateLimitPlugin.Config.OmitConsumer = "gateway"
	return nil
}

func setOptions(rateLimitPlugin *plugin.RateLimitPlugin, options gatewayv1.RateLimitOptions) *plugin.RateLimitPlugin {
	rateLimitPlugin.Config.HideClientHeaders = options.HideClientHeaders
	rateLimitPlugin.Config.FaultTolerant = options.FaultTolerant
	return rateLimitPlugin
}
