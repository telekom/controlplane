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
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
)

var _ features.KongFeature = &ExternalIDPFeature{}

// defaultKey for provider (exposure) config.
// Used as a fallback in Jumper if no consumer key is found
const DefaultProviderKey = plugin.ConsumerId("default")

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

// IsUsed checks if the ExternalIDP feature is used in the route.
// It can either be used as a primary route feature or as a failover security feature.
func (f *ExternalIDPFeature) IsUsed(ctx context.Context, builder features.KongFeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	isPrimaryRoute := route.Spec.Type == gatewayv1.RouteTypePrimary
	isConfigured := false

	if HasFailoverSecurity(route) {
		isConfigured = route.Spec.Traffic.Failover.Security.HasM2MExternalIDP()
	}

	if isPrimaryRoute && HasM2MExternalIdp(route) {
		isConfigured = true
	}

	return !route.Spec.PassThrough && isConfigured
}

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.KongFeatureBuilder) (err error) {
	rtpPlugin := builder.RequestTransformerPlugin()
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	jumperConfig := builder.JumperConfig()

	// Depending on the context (primary or failover route), we need to use different security settings.
	// If the route is a failover secondary route, we use the failover security settings
	// Otherwise, we use the primary route security settings.
	security := route.Spec.Security
	if HasFailoverSecurity(route) {
		security = route.Spec.Traffic.Failover.Security
	}

	rtpPlugin.Config.Append.AddHeader("token_endpoint", security.M2M.ExternalIDP.TokenEndpoint)

	// Provider
	if security.HasM2MExternalIDP() && security.M2M.ExternalIDP.Client != nil {
		err = applyOauth(ctx, DefaultProviderKey, jumperConfig, security.M2M.ExternalIDP.Client, security.M2M.ExternalIDP, security.M2M.Scopes)
		if err != nil {
			return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
		}
	} else if security.HasM2MExternalIDP() && security.M2M.ExternalIDP.Basic != nil {
		err = applyBasic(ctx, DefaultProviderKey, jumperConfig, security.M2M.ExternalIDP.Basic, security.M2M.ExternalIDP, security.M2M.Scopes)
		if err != nil {
			return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
		}
	}

	// Consumers
	for _, consumer := range builder.GetAllowedConsumers() {
		if consumer.HasM2MClient() {
			err = applyOauth(ctx, plugin.ConsumerId(consumer.Spec.ConsumerName), jumperConfig, consumer.Spec.Security.M2M.Client, security.M2M.ExternalIDP, consumer.Spec.Security.M2M.Scopes)
			if err != nil {
				return errors.Wrapf(err, "cannot get consumer secret for consumer %s in route %s", consumer.Spec.ConsumerName, route.Name)
			}
		} else if consumer.HasM2MBasic() {
			err = applyBasic(ctx, plugin.ConsumerId(consumer.Spec.ConsumerName), jumperConfig, consumer.Spec.Security.M2M.Basic, security.M2M.ExternalIDP, consumer.Spec.Security.M2M.Scopes)
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
	clientKey := client.ClientKey
	refreshToken := client.RefreshToken

	if clientKey != "" {
		clientKey, err = secretManagerApi.Get(ctx, clientKey)
		if err != nil {
			return in, err
		}

		in.ClientKey = clientKey

	} else if secret != "" {
		secret, err = secretManagerApi.Get(ctx, secret)
		if err != nil {
			return in, err
		}

		in.ClientSecret = secret
	}

	if refreshToken != "" {
		refreshToken, err = secretManagerApi.Get(ctx, refreshToken)
		if err != nil {
			return in, err
		}

		in.RefreshToken = refreshToken
	}

	if len(scopes) > 0 {
		in.Scopes = strings.Join(scopes, " ")
	}

	tokenRequest, err := tokenRequestToJumper(providerSettings.TokenRequest)
	if err != nil {
		return in, err
	}
	in.TokenRequest = tokenRequest
	in.GrantType = string(providerSettings.GrantType)

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
	in.GrantType = string(providerSettings.GrantType)

	return in, nil
}

// tokenRequestToJumper converts CRD tokenRequest values to the values expected by the Jumper plugin.
func tokenRequestToJumper(value gatewayv1.TokenRequestMethod) (string, error) {
	switch value {
	case gatewayv1.TokenRequestClientSecretBasic:
		return "header", nil
	case gatewayv1.TokenRequestClientSecretPost:
		return "body", nil
	default:
		return "", fmt.Errorf("unsupported tokenRequest value %q", value)
	}
}
