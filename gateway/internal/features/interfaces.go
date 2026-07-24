// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features

import (
	"context"

	"google.golang.org/protobuf/types/known/anypb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

type FeatureBuilder interface {
	GetRoute() (*gatewayv1.Route, bool)
	GetConsumer() (*gatewayv1.Consumer, bool)
	GetGateway() *gatewayv1.Gateway
	GetAllowedConsumers() []*gatewayv1.ConsumeRoute
	AddAllowedConsumers(...*gatewayv1.ConsumeRoute)

	Build(context.Context) error
	BuildForConsumer(context.Context) error
}

type FeatureInfo interface {
	// Name of the feature
	Name() gatewayv1.FeatureType
	// Priority of this feature in the feature-chain
	// The higher the priority value, the later the feature is applied. Or, in other words, the lower the priority value, the earlier the feature is applied.
	// Features can have a relative priority to other features to indicate that some features should be applied before or after others.
	Priority() int
}

type Feature[T FeatureBuilder] interface {
	FeatureInfo

	// IsUsed checks if the feature is already used in the current builder-context before applying it.
	// The business-context (context.Context) is available to check/create runtime components such as the environment or loggers.
	IsUsed(ctx context.Context, builder T) bool
	// Apply applies the feature to the current builder-context
	// It may modify the plugins and upstream of the builder-context
	// The business-context (context.Context) is available to check/create runtime components such as the environment or loggers.
	Apply(ctx context.Context, builder T) error
}

type KongFeature = Feature[KongFeatureBuilder]

type KongFeatureBuilder interface {
	FeatureBuilder

	EnableFeature(f KongFeature)

	SetUpstream(client.Upstream)
	RequestTransformerPlugin() *plugin.RequestTransformerPlugin
	AclPlugin() *plugin.AclPlugin
	JwtPlugin() *plugin.JwtPlugin
	RateLimitPluginRoute() *plugin.RateLimitPlugin
	RateLimitPluginConsumeRoute(*gatewayv1.ConsumeRoute) *plugin.RateLimitPlugin
	JumperConfig() *plugin.JumperConfig
	RoutingConfigs() *plugin.RoutingConfigs
	IpRestrictionPlugin() *plugin.IpRestrictionPlugin

	GetKongClient() client.KongClient
}

type EnvoyFeature = Feature[EnvoyFeatureBuilder]

type EnvoyFeatureBuilder interface {
	FeatureBuilder

	EnableFeature(f EnvoyFeature)

	// AddHTTPFilter contributes one HTTP filter (typed config marshaled to Any)
	// to the listener's filter chain. Features add filters in the order their
	// Apply runs (feature priority order); the builder inserts them before the
	// terminal router filter. name must be the canonical Envoy filter name.
	AddHTTPFilter(name string, typedConfig *anypb.Any)
}
