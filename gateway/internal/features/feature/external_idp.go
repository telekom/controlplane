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

var _ features.Feature = &ExternalIDPFeature{}

// ExternalIDPFeature takes precedence over CustomScopesFeature
type ExternalIDPFeature struct {
	priority int
}

var InstanceExternalIDPFeature = &ExternalIDPFeature{
	priority: InstanceCustomScopesFeature.priority - 1,
}

func (f *ExternalIDPFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeExternalIDP
}

func (f *ExternalIDPFeature) Priority() int {
	return f.priority
}

func (f *ExternalIDPFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	hasExternalIdpConfigured := true

	return !route.Spec.PassThrough && hasExternalIdpConfigured && !route.IsProxy()
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()

	// TODO: get from route or somewhere
	rtpPlugin.Config.Append.AddHeader("token_endpoint", "https://example.com/token")

	jumperConfig := builder.JumperConfig()

	for _, consumer := range builder.GetAllowedConsumers() { // TODO: implement

		// TODO: handle default
		jumperConfig.OAuth[plugin.ConsumerId("default")] = plugin.OauthCredentials{
			ClientId:     "default_client_id",
			ClientSecret: "default_client_secret",
		}

		// TODO: implement
		jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = plugin.OauthCredentials{
			Scopes:       "custom_scope",
			ClientId:     "client_id",
			ClientSecret: "client_secret",
		}
	}

	return nil
}
