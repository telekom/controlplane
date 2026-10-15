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

var _ features.Feature = &ExternalIDPFeature{}

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
func (f *ExternalIDPFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
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

func (f *ExternalIDPFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
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
	idp := security.M2M.ExternalIDP
	if ok, err := hasCredentialsForGrant(idp.Client, idp.Basic, idp.GrantType); err != nil {
		return errors.Wrapf(err, "cannot identify grantType for %s", route.Name)
	} else if ok {
		if err = applyCredentials(ctx, DefaultProviderKey, jumperConfig, idp, idp.Client, idp.Basic, security.M2M.Scopes, idp.GrantType); err != nil {
			return errors.Wrapf(err, "cannot get provider secret for route %s", route.Name)
		}
	}

	// Consumers
	for _, consumer := range builder.GetAllowedConsumers() {
		if err = applyConsumer(ctx, jumperConfig, consumer, idp); err != nil {
			return errors.Wrapf(err, "cannot get consumer secret for consumer %s in route %s", consumer.Spec.ConsumerName, route.Name)
		}
	}

	return nil
}

// hasCredentialsForGrant reports whether the credentials can build a token request for the grant type.
// Jumper sends every configured credential together with the grant type and does not check if they match.
func hasCredentialsForGrant(client *gatewayv1.OAuth2ClientCredentials, basic *gatewayv1.BasicAuthCredentials, grantType gatewayv1.GrantType) (bool, error) {
	switch grantType {
	case gatewayv1.GrantTypePassword:
		return basic != nil, nil
	case gatewayv1.GrantTypeRefreshToken:
		return client != nil && client.RefreshToken != "", nil
	case gatewayv1.GrantTypeAuthorizationCode, gatewayv1.GrantTypeClientCredentials:
		return client != nil, nil
	default:
		return false, fmt.Errorf("unknown grantType: %s", grantType)
	}
}

// applyCredentials publishes the credentials used for the grant type. Username/password are only used with the
// password grant, client credentials authenticate the client for every grant type.
func applyCredentials(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, providerSettings *gatewayv1.ExternalIdentityProvider, client *gatewayv1.OAuth2ClientCredentials, basic *gatewayv1.BasicAuthCredentials, scopes []string, grantType gatewayv1.GrantType) error {
	if client != nil {
		if err := applyOauth(ctx, key, jumperConfig, client, providerSettings, scopes, grantType); err != nil {
			return err
		}
	}
	if grantType == gatewayv1.GrantTypePassword {
		return applyBasic(ctx, key, jumperConfig, basic, scopes, grantType)
	}
	return nil
}

func applyConsumer(ctx context.Context, jumperConfig *plugin.JumperConfig, consumer *gatewayv1.ConsumeRoute, providerSettings *gatewayv1.ExternalIdentityProvider) error {
	// Backend Basic credentials are handled by the BasicAuthFeature.
	if !consumer.HasM2M() || usesBackendBasicAuth(consumer) {
		return nil
	}
	key := plugin.ConsumerId(consumer.Spec.ConsumerName)
	consumerM2M := consumer.Spec.Security.M2M
	grantType := providerSettings.GrantType
	if consumerM2M.GrantType != "" {
		grantType = consumerM2M.GrantType
	}

	if ok, err := hasCredentialsForGrant(consumerM2M.Client, consumerM2M.Basic, grantType); err != nil {
		return err
	} else if ok {
		return applyCredentials(ctx, key, jumperConfig, providerSettings, consumerM2M.Client, consumerM2M.Basic, consumerM2M.Scopes, grantType)
	}

	// Without own credentials for the grant type, the consumer requests its scopes with the provider credentials.
	// Jumper completes a scopes-only entry from the provider default, so it needs one and a grant type.
	_, hasProviderDefault := jumperConfig.OAuth[DefaultProviderKey]
	if hasProviderDefault && len(consumerM2M.Scopes) > 0 {
		jumperConfig.OAuth[key] = plugin.OauthCredentials{
			Scopes:    strings.Join(consumerM2M.Scopes, " "),
			GrantType: string(providerSettings.GrantType),
		}
	}
	return nil
}

func applyOauth(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, client *gatewayv1.OAuth2ClientCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, scopes []string, grantType gatewayv1.GrantType) error {
	current := jumperConfig.OAuth[key]
	oauth, err := extendOauth(ctx, &current, providerSettings, client, scopes, grantType)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = oauth

	return nil
}

func extendOauth(ctx context.Context, current *plugin.OauthCredentials, providerSettings *gatewayv1.ExternalIdentityProvider, client *gatewayv1.OAuth2ClientCredentials, scopes []string, grantType gatewayv1.GrantType) (plugin.OauthCredentials, error) {
	in := *current
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
	in.GrantType = string(grantType)

	return in, nil
}

func applyBasic(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, basic *gatewayv1.BasicAuthCredentials, scopes []string, grantType gatewayv1.GrantType) error {
	current := jumperConfig.OAuth[key]
	basicAuth, err := extendBasic(ctx, &current, basic, scopes, grantType)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = basicAuth
	return nil
}

func extendBasic(ctx context.Context, current *plugin.OauthCredentials, basic *gatewayv1.BasicAuthCredentials, scopes []string, grantType gatewayv1.GrantType) (plugin.OauthCredentials, error) {
	in := *current
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
	in.GrantType = string(grantType)

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
