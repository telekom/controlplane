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

func (f *ExternalIDPFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route := builder.GetRoute()
	if route == nil {
		return false
	}

	if !route.Spec.HasM2MExternalIdp() {
		return false
	}

	hasExternalTokenEndpoint := route.Spec.Security.M2M.ExternalIDP.TokenEndpoint != ""

	return !route.Spec.PassThrough && hasExternalTokenEndpoint && !route.IsProxy()
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()
	route := builder.GetRoute()
	jumperConfig := builder.JumperConfig()

	upstream := route.Spec.Upstreams[0]
	builder.SetUpstream(upstream)
	rtpPlugin.Config.Append.AddHeader("token_endpoint", route.Spec.Security.M2M.ExternalIDP.TokenEndpoint)

	providerOauth, err := extendOauth(ctx, jumperConfig.OAuth[defaultProviderKey], route.Spec.Security.M2M.ExternalIDP, route.Spec.Security.M2M.ExternalIDP.Client)
	if err != nil {
		return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
	}
	jumperConfig.OAuth[defaultProviderKey] = providerOauth

	for _, consumer := range builder.GetAllowedConsumers() {
		if !consumer.Spec.HasM2MExternalIdp() {
			continue
		}
		oauth, err := extendOauth(ctx, jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)], route.Spec.Security.M2M.ExternalIDP, consumer.Spec.Security.M2M.ExternalIDP.Client)
		if err != nil {
			return errors.Wrapf(err, "cannot get consumer secret for consumer %s", consumer.Spec.ConsumerName)
		}
		jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = oauth
	}

	return nil
}

func extendOauth(ctx context.Context, in plugin.OauthCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, client *gatewayv1.OAuth2ClientCredentials) (plugin.OauthCredentials, error) {
	var err error

	in.ClientId = client.ClientId
	secret := client.ClientSecret
	if secret != "" {
		secret, err = secretManagerApi.Get(ctx, secret)
		if err != nil {
			return in, err
		}
	}
	in.ClientSecret = secret

	if in.Scopes == "" && len(client.Scopes) > 0 {
		in.Scopes = strings.Join(client.Scopes, " ")
	}

	in.TokenRequest = providerSettings.TokenRequest
	in.GrantType = providerSettings.GrantType

	return in, nil
}
