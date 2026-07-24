// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kong

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ features.KongFeatureBuilder = &Builder{}

type Builder struct {
	// kc is the Kong client used to interact with the Kong Gateway
	kc client.KongClient

	// AllowedConsumers are the consumers that are allowed to consume the passed route
	AllowedConsumers []*gatewayv1.ConsumeRoute

	Route    *gatewayv1.Route
	Consumer *gatewayv1.Consumer

	Gateway *gatewayv1.Gateway

	Upstream client.Upstream

	// Plugins that are stored in the builder and to be configured by the builders for the route
	Plugins map[string]client.CustomPlugin

	// jumperConfig is a special plugin that is always required by the API Gateway
	jumperConfig *plugin.JumperConfig

	// routingConfig is used to configure the routing behavior of the API Gateway in case of failover
	routingConfigs *plugin.RoutingConfigs

	// Features that are enabled for this builder
	Features map[gatewayv1.FeatureType]features.KongFeature
}

func (b *Builder) GetKongClient() client.KongClient {
	return b.kc
}

var NewFeatureBuilder = func(kc client.KongClient, route *gatewayv1.Route, consumer *gatewayv1.Consumer, gateway *gatewayv1.Gateway) features.KongFeatureBuilder {
	return &Builder{
		kc: kc,

		AllowedConsumers: []*gatewayv1.ConsumeRoute{},
		Route:            route,
		Consumer:         consumer,
		Gateway:          gateway,

		Plugins:  map[string]client.CustomPlugin{},
		Features: map[gatewayv1.FeatureType]features.KongFeature{},
	}
}

func (b *Builder) EnableFeature(f features.KongFeature) {
	b.Features[f.Name()] = f
}

func (b *Builder) GetRoute() (*gatewayv1.Route, bool) {
	if b.Route == nil {
		return nil, false
	}
	return b.Route, true
}
func (b *Builder) GetConsumer() (*gatewayv1.Consumer, bool) {
	if b.Consumer == nil {
		return nil, false
	}
	return b.Consumer, true
}

func (b *Builder) GetGateway() *gatewayv1.Gateway {
	return b.Gateway
}

func (b *Builder) GetAllowedConsumers() []*gatewayv1.ConsumeRoute {
	return b.AllowedConsumers
}

func (b *Builder) AddAllowedConsumers(consumers ...*gatewayv1.ConsumeRoute) {
	b.AllowedConsumers = append(b.AllowedConsumers, consumers...)
}

func (b *Builder) RequestTransformerPlugin() *plugin.RequestTransformerPlugin {
	var rtpPlugin *plugin.RequestTransformerPlugin

	if p, ok := b.Plugins["request-transformer"]; ok {
		rtpPlugin, ok = p.(*plugin.RequestTransformerPlugin)
		if !ok {
			panic("plugin is not a RequestTransformerPlugin")
		}
	} else {
		rtpPlugin = plugin.RequestTransformerPluginFromRoute(b.Route)
		b.Plugins["request-transformer"] = rtpPlugin
	}

	return rtpPlugin
}

func (b *Builder) AclPlugin() *plugin.AclPlugin {
	var aclPlugin *plugin.AclPlugin

	if p, ok := b.Plugins["acl"]; ok {
		aclPlugin, ok = p.(*plugin.AclPlugin)
		if !ok {
			panic("plugin is not a AclPlugin")
		}
	} else {
		aclPlugin = plugin.AclPluginFromRoute(b.Route)
		b.Plugins["acl"] = aclPlugin
	}

	return aclPlugin
}

func (b *Builder) JwtPlugin() *plugin.JwtPlugin {
	var jwtPlugin *plugin.JwtPlugin

	if p, ok := b.Plugins["jwt"]; ok {
		jwtPlugin, ok = p.(*plugin.JwtPlugin)
		if !ok {
			panic("plugin is not a JwtPlugin")
		}
	} else {
		jwtPlugin = plugin.JwtPluginFromRoute(b.Route)
		b.Plugins["jwt"] = jwtPlugin
	}

	return jwtPlugin
}

func (b *Builder) RateLimitPluginRoute() *plugin.RateLimitPlugin {
	var rateLimitPlugin *plugin.RateLimitPlugin

	if p, ok := b.Plugins["rate-limiting"]; ok {
		rateLimitPlugin, ok = p.(*plugin.RateLimitPlugin)
		if !ok {
			panic("plugin is not a RateLimitPlugin")
		}
	} else {
		rateLimitPlugin = plugin.RateLimitPluginFromRoute(b.Route)
		b.Plugins["rate-limiting"] = rateLimitPlugin
	}

	return rateLimitPlugin
}

func (b *Builder) RateLimitPluginConsumeRoute(consumeRoute *gatewayv1.ConsumeRoute) *plugin.RateLimitPlugin {
	var rateLimitPlugin *plugin.RateLimitPlugin
	consumerName := consumeRoute.Spec.ConsumerName

	if p, ok := b.Plugins["rate-limiting-consumer--"+consumerName]; ok {
		rateLimitPlugin, ok = p.(*plugin.RateLimitPlugin)
		if !ok {
			panic("plugin is not a RateLimitPlugin")
		}
	} else {
		rateLimitPlugin = plugin.RateLimitPluginFromConsumeRoute(consumeRoute)
		b.Plugins["rate-limiting-consumer--"+consumerName] = rateLimitPlugin
	}

	return rateLimitPlugin
}

func (b *Builder) JumperConfig() *plugin.JumperConfig {
	if b.jumperConfig == nil {
		b.jumperConfig = plugin.NewJumperConfig()
	}
	return b.jumperConfig
}

func (b *Builder) RoutingConfigs() *plugin.RoutingConfigs {
	if b.routingConfigs == nil {
		b.routingConfigs = &plugin.RoutingConfigs{}
	}
	return b.routingConfigs
}

func (b *Builder) IpRestrictionPlugin() *plugin.IpRestrictionPlugin {
	var ipRestrictionPlugin *plugin.IpRestrictionPlugin

	if p, ok := b.Plugins["ip-restriction"]; ok {
		ipRestrictionPlugin, ok = p.(*plugin.IpRestrictionPlugin)
		if !ok {
			panic("plugin is not a IpRestrictionPlugin")
		}
	} else {
		ipRestrictionPlugin = plugin.IpRestrictionPluginFromConsumer(b.Consumer)
		b.Plugins["ip-restriction"] = ipRestrictionPlugin
	}

	return ipRestrictionPlugin
}

func (b *Builder) SetUpstream(upstream client.Upstream) {
	b.Upstream = upstream
}

func (b *Builder) Build(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("features.builder").WithValues("route", b.Route.Name)
	if b.Route == nil {
		return features.ErrNoRoute
	}

	for _, f := range features.SortFeatures(features.ToSlice(b.Features)) {
		if f.IsUsed(ctx, b) {
			log.V(1).Info("Applying feature", "name", f.Name())
			err := f.Apply(ctx, b)
			if err != nil {
				return err
			}
		} else {
			log.V(1).Info("Feature is not used", "name", f.Name())
		}
	}

	if b.Upstream == nil {
		return errors.New("upstream is not set")
	}

	// In case a plugin was used before but is not used anymore, we need to remove it
	b.Route.Status.Properties = map[string]string{}

	// Ensure that the Routing and JumperConfig are set last
	// ! We must ensure that the default (empty) value is null. Otherwise, Jumper will not work properly.
	if b.routingConfigs != nil {
		b.RequestTransformerPlugin().Config.Append.AddHeader(plugin.RoutingConfigKey, plugin.ToBase64OrDie(b.routingConfigs))
	} else if b.jumperConfig != nil {
		b.RequestTransformerPlugin().Config.Append.AddHeader(plugin.JumperConfigKey, plugin.ToBase64OrDie(b.jumperConfig))
	}

	err := b.kc.CreateOrReplaceRoute(ctx, b.Route, b.Upstream)
	if err != nil {
		return errors.Wrap(err, "failed to create or replace route")
	}

	for pn, p := range b.Plugins {
		_, err = b.kc.CreateOrReplacePlugin(ctx, p)
		if err != nil {
			return errors.Wrapf(err, "failed to create or replace plugin %s", pn)
		}
	}

	err = b.kc.CleanupPlugins(ctx, b.Route, nil, features.ToSlice(b.Plugins))
	if err != nil {
		return errors.Wrap(err, "failed to cleanup plugins")
	}

	return nil
}

func (b *Builder) BuildForConsumer(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("features.builder").WithValues("consumer", b.Consumer.Name)
	if b.Consumer == nil {
		return features.ErrNoConsumer
	}

	for _, f := range features.SortFeatures(features.ToSlice(b.Features)) {
		if f.IsUsed(ctx, b) {
			log.V(1).Info("Applying feature", "name", f.Name())
			err := f.Apply(ctx, b)
			if err != nil {
				return err
			}
		} else {
			log.V(1).Info("Feature is not used", "name", f.Name())
		}
	}

	// In case a plugin was used before but is not used anymore, we need to remove it
	b.Consumer.Status.Properties = map[string]string{}

	_, err := b.kc.CreateOrReplaceConsumer(ctx, b.Consumer)
	if err != nil {
		return errors.Wrap(err, "failed to create or replace consumer")
	}

	for pn, p := range b.Plugins {
		_, err = b.kc.CreateOrReplacePlugin(ctx, p)
		if err != nil {
			return errors.Wrapf(err, "failed to create or replace plugin %s", pn)
		}
	}

	err = b.kc.CleanupPlugins(ctx, nil, b.Consumer, features.ToSlice(b.Plugins))
	if err != nil {
		return errors.Wrap(err, "failed to cleanup plugins")
	}

	return nil

}
