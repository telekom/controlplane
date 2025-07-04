// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/util"
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
	upstream := getFirstUpstreamWithIDPConfig(&route.Spec.Upstreams)
	if upstream == nil {
		return false
	}
	hasExternalTokenEndpoint := upstream.Security.M2M.ExternalIDP.TokenEndpoint != ""

	return !route.Spec.PassThrough && hasExternalTokenEndpoint && !route.IsProxy()
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()
	route := builder.GetRoute()
	jumperConfig := builder.JumperConfig()

	upstream := getFirstUpstreamWithIDPConfig(&route.Spec.Upstreams)
	if upstream == nil {
		return fmt.Errorf("no upstream with external IDP config found for route %s", route.Name)
	}
	builder.SetUpstream(upstream)
	rtpPlugin.Config.Append.AddHeader("token_endpoint", upstream.Security.M2M.ExternalIDP.TokenEndpoint)

	providerOauth, err := extendOauth(ctx, jumperConfig.OAuth[defaultProviderKey], upstream.Security.M2M.ExternalIDP, upstream.Security.M2M.ExternalIDP.Client)
	if err != nil {
		return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
	}
	jumperConfig.OAuth[defaultProviderKey] = providerOauth

	for _, consumer := range builder.GetAllowedConsumers() {
		if !util.HasM2MExternalIdpClientConsumeRoute(consumer) {
			continue
		}
		oauth, err := extendOauth(ctx, jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)], upstream.Security.M2M.ExternalIDP, consumer.Spec.Security.M2M.ExternalIDP.Client)
		if err != nil {
			return errors.Wrapf(err, "cannot get consumer secret for consumer %s", consumer.Spec.ConsumerName)
		}
		jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)] = oauth
	}

	return nil
}

func getFirstUpstreamWithIDPConfig(upstreams *[]gatewayv1.Upstream) *gatewayv1.Upstream {
	for i := range *upstreams {
		if util.HasM2MExternalIdpUpstream(&(*upstreams)[i]) {
			if (*upstreams)[i].Security.M2M.ExternalIDP != nil {
				return &(*upstreams)[i]
			}
		}
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
