// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"slices"

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

	if route.Spec.PassThrough {
		return false
	}

	anyConsumerHasRateLimiting := slices.ContainsFunc(builder.GetAllowedConsumers(), func(consumer *gatewayv1.ConsumeRoute) bool {
		// Only consider consumers that are directly linked to the route
		// and have a rate-limiting configuration.
		return consumer.Spec.Route.Equals(route) && consumer.HasTrafficRateLimit()
	})

	return route.HasRateLimit() || anyConsumerHasRateLimiting
}

func (f *RateLimitFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	if !route.IsProxy() && route.HasRateLimit() {
		// If this is a primary-route, we need to apply the rate-limiting plugin for the provider-side

		rateLimitPlugin := builder.RateLimitPluginRoute()
		if err = setCommonConfigs(ctx, rateLimitPlugin, builder.GetGateway()); err != nil {
			return err
		}
		rateLimitPlugin.Config.Limits = plugin.Limits{
			Service: &plugin.LimitConfig{
				Second: route.Spec.Traffic.RateLimit.Limits.Second,
				Minute: route.Spec.Traffic.RateLimit.Limits.Minute,
				Hour:   route.Spec.Traffic.RateLimit.Limits.Hour,
			},
		}
		setOptions(rateLimitPlugin, &route.Spec.Traffic.RateLimit.Options)
	}

	for _, allowedConsumer := range builder.GetAllowedConsumers() {
		routeRef := allowedConsumer.Spec.Route
		if !routeRef.Equals(route) {
			// Only configure the rate-limiting plugin for routes which are directly used by this consumer.
			// There is no need to apply all consumer-rate-limiting plugins to the primary-route
			continue
		}
		if allowedConsumer.HasTrafficRateLimit() {
			// If the consumer has a rate-limiting configuration, we need to apply the rate-limiting plugin for the consumer-side
			rateLimitPlugin := builder.RateLimitPluginConsumeRoute(allowedConsumer)
			if err = setCommonConfigs(ctx, rateLimitPlugin, builder.GetGateway()); err != nil {
				return err
			}

			rateLimitPlugin.Config.Limits.Consumer = &plugin.LimitConfig{
				Second: allowedConsumer.Spec.Traffic.RateLimit.Limits.Second,
				Minute: allowedConsumer.Spec.Traffic.RateLimit.Limits.Minute,
				Hour:   allowedConsumer.Spec.Traffic.RateLimit.Limits.Hour,
			}

			// If this is a primary-route, we need to apply the provider rate-limiting config
			// to the consumer rate-limiting plugin as well.
			if !route.IsProxy() && route.HasRateLimit() {
				rateLimitPlugin.Config.Limits.Service = &plugin.LimitConfig{
					Second: route.Spec.Traffic.RateLimit.Limits.Second,
					Minute: route.Spec.Traffic.RateLimit.Limits.Minute,
					Hour:   route.Spec.Traffic.RateLimit.Limits.Hour,
				}

				setOptions(rateLimitPlugin,
					&route.Spec.Traffic.RateLimit.Options,
				)
			} else if route.HasRateLimit() {
				setOptions(rateLimitPlugin, &route.Spec.Traffic.RateLimit.Options)
			}
		}
	}

	return nil
}

func setCommonConfigs(ctx context.Context, rateLimitPlugin *plugin.RateLimitPlugin, gateway *gatewayv1.Gateway) error {
	rateLimitPlugin.Config.Policy = plugin.PolicyRedis
	redisPassword, err := secretManagerApi.Get(ctx, gateway.Spec.Redis.Password)
	if err != nil {
		return errors.Wrapf(err, "cannot get redis password for gateway %s", gateway.GetName())
	}
	rateLimitPlugin.Config.RedisConfig = plugin.RedisConfig{
		Host:     gateway.Spec.Redis.Host,
		Port:     gateway.Spec.Redis.Port,
		Ssl:      gateway.Spec.Redis.EnableTLS,
		Password: redisPassword,
	}
	rateLimitPlugin.Config.OmitConsumer = "gateway"
	return nil
}

// setOptions applies additional options to the rate limit plugin configuration.
// Options should be in order of precedence, with the last one having the highest priority.
func setOptions(rateLimitPlugin *plugin.RateLimitPlugin, options ...*gatewayv1.RateLimitOptions) {
	for _, option := range options {
		if option == nil {
			continue
		}
		rateLimitPlugin.Config.HideClientHeaders = option.HideClientHeaders
		rateLimitPlugin.Config.FaultTolerant = option.FaultTolerant
	}
}
