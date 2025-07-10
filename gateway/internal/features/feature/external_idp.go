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

	if !route.HasM2MExternalIdp() {
		return false
	}

	hasExternalTokenEndpoint := route.Spec.Security.M2M.ExternalIDP.TokenEndpoint != ""

	return !route.Spec.PassThrough && hasExternalTokenEndpoint && !route.IsProxy()
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()
	route := builder.GetRoute()
	jumperConfig := builder.JumperConfig()

	rtpPlugin.Config.Append.AddHeader("token_endpoint", route.Spec.Security.M2M.ExternalIDP.TokenEndpoint)

	// Provider
	if route.HasM2MExternalIdpClient() {
		err = applyOauth(ctx, defaultProviderKey, jumperConfig, route.Spec.Security.M2M.ExternalIDP.Client, route.Spec.Security.M2M.ExternalIDP, route.Spec.Security.M2M.Scopes)
		if err != nil {
			return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
		}
	} else if route.HasM2MExternalIdpBasic() {
		err = applyBasic(ctx, defaultProviderKey, jumperConfig, route.Spec.Security.M2M.ExternalIDP.Basic, route.Spec.Security.M2M.ExternalIDP, route.Spec.Security.M2M.Scopes)
		if err != nil {
			return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
		}
	}

	// Consumers
	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.HasM2MClient() {
			err = applyOauth(ctx, plugin.ConsumerId(consumer.Spec.ConsumerName), jumperConfig, consumer.Spec.Security.M2M.Client, route.Spec.Security.M2M.ExternalIDP, consumer.Spec.Security.M2M.Scopes)
			if err != nil {
				return errors.Wrapf(err, "cannot get consumer secret for consumer %s in route %s", consumer.Spec.ConsumerName, route.Name)
			}
		} else if consumer.HasM2MBasic() {
			err = applyBasic(ctx, plugin.ConsumerId(consumer.Spec.ConsumerName), jumperConfig, consumer.Spec.Security.M2M.Basic, route.Spec.Security.M2M.ExternalIDP, consumer.Spec.Security.M2M.Scopes)
			if err != nil {
				return errors.Wrapf(err, "cannot get consumer secret for consumer %s in route %s", consumer.Spec.ConsumerName, route.Name)
			}
		}
	}

	return nil
}

func applyOauth(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, client *gatewayv1.OAuth2ClientCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, scopes []string) error {
	oauth, err := extendOauth(ctx, jumperConfig.OAuth[key], providerSettings, client, scopes)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = oauth

	return nil
}

func extendOauth(ctx context.Context, in plugin.OauthCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, client *gatewayv1.OAuth2ClientCredentials, scopes []string) (plugin.OauthCredentials, error) {
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

	if len(scopes) > 0 {
		in.Scopes = strings.Join(scopes, " ")
	}

	in.TokenRequest = providerSettings.TokenRequest
	in.GrantType = providerSettings.GrantType

	return in, nil
}

func applyBasic(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, basic *gatewayv1.BasicAuthCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, scopes []string) error {
	basicAuth, err := extendBasic(ctx, jumperConfig.OAuth[key], providerSettings, basic, scopes)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = basicAuth
	return nil
}

func extendBasic(ctx context.Context, in plugin.OauthCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, basic *gatewayv1.BasicAuthCredentials, scopes []string) (plugin.OauthCredentials, error) {
	var err error

	in.Username = basic.Username
	password := basic.Password
	if password != "" {
		password, err = secretManagerApi.Get(ctx, password)
		if err != nil {
			return in, err
		}
	}

	if len(scopes) > 0 {
		in.Scopes = strings.Join(scopes, " ")
	}

	in.Password = password
	in.GrantType = providerSettings.GrantType

	return in, nil
}
