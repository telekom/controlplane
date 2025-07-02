// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
)

var _ features.Feature = &ExternalIDPFeature{}

// defaultKey for provider (exposure) config.
// Used as a fallback in Jumper if no consumer key is found
const defaultProviderKey = plugin.ConsumerId("default")

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

func (f *ExternalIDPFeature) IsUsed(_ context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	hasExternalTokenEndpoint := false
	for i := range route.Spec.Upstreams {
		if route.Spec.Upstreams[i].Security != nil {
			if route.Spec.Upstreams[i].Security.M2M != nil {
				if route.Spec.Upstreams[i].Security.M2M.ExternalIDPConfig != nil {
					if route.Spec.Upstreams[i].Security.M2M.ExternalIDPConfig.TokenEndpoint != "" {
						hasExternalTokenEndpoint = true
						break
					}
				}
			}
		}
	}
	return !route.Spec.PassThrough && hasExternalTokenEndpoint && !route.IsProxy()
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()
	route := builder.GetRoute()
	jumperConfig := builder.JumperConfig()
	var upstream gatewayv1.Upstream

	for i := range route.Spec.Upstreams {
		if route.Spec.Upstreams[i].Security.M2M.ExternalIDPConfig.TokenEndpoint != "" {
			rtpPlugin.Config.Append.AddHeader("token_endpoint", route.Spec.Upstreams[i].Security.M2M.ExternalIDPConfig.TokenEndpoint)
			upstream = route.Spec.Upstreams[i]
			builder.SetUpstream(upstream)
			break
		}
	}

	providerSecret := upstream.Security.M2M.Client.ClientSecret
	if providerSecret != "" {
		providerSecret, err = secretManagerApi.Get(ctx, providerSecret)
		if err != nil {
			return errors.Wrapf(err, "failed to get provider secret for upstream %s", upstream.IssuerUrl)
		}

	}

	providerOauth := jumperConfig.OAuth[defaultProviderKey]
	if providerOauth.Scopes == "" && len(upstream.Security.M2M.Client.Scopes) > 0 {
		providerOauth.Scopes = strings.Join(upstream.Security.M2M.Client.Scopes, " ")
	}
	providerOauth.ClientId = upstream.Security.M2M.Client.ClientId
	providerOauth.ClientSecret = providerSecret
	providerOauth.TokenRequest = upstream.Security.M2M.ExternalIDPConfig.TokenRequest
	providerOauth.GrantType = upstream.Security.M2M.ExternalIDPConfig.GrantType

	jumperConfig.OAuth[defaultProviderKey] = providerOauth

	for _, consumer := range builder.GetAllowedConsumers() {
		consumerSecret := consumer.Spec.Security.M2M.Client.ClientSecret
		if consumerSecret != "" {
			consumerSecret, err = secretManagerApi.Get(ctx, consumerSecret)
			if err != nil {
				return errors.Wrapf(err, "cannot get consumer secret for consumer %s", consumer.Spec.ConsumerName)
			}
		}

		consumerOauth := jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)]
		if consumerOauth.Scopes == "" && len(consumer.Spec.Security.M2M.Client.Scopes) > 0 {
			consumerOauth.Scopes = strings.Join(consumer.Spec.Security.M2M.Client.Scopes, " ")
		}
		consumerOauth.ClientId = consumer.Spec.Security.M2M.Client.ClientId
		consumerOauth.ClientSecret = consumerSecret
		consumerOauth.TokenRequest = consumer.Spec.Security.M2M.ExternalIDPConfig.TokenRequest
		consumerOauth.GrantType = consumer.Spec.Security.M2M.ExternalIDPConfig.GrantType

		jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = consumerOauth
	}

	return nil
}
