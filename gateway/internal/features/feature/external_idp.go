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

// idpContext holds the credentials and token request settings of the provider or a consumer
// that are used to build one Jumper OAuth entry.
type idpContext struct {
	client       *gatewayv1.OAuth2ClientCredentials
	basic        *gatewayv1.BasicAuthCredentials
	scopes       []string
	grantType    gatewayv1.GrantType
	tokenRequest gatewayv1.TokenRequestMethod
}

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
	provider := newProviderIdpContext(idp, security.M2M.Scopes)
	hasProviderCredentials, err := hasCredentialsForGrant(provider)
	if err != nil {
		return errors.Wrapf(err, "cannot identify grantType for %s", route.Name)
	}
	if hasProviderCredentials {
		if err = applyCredentials(ctx, DefaultProviderKey, jumperConfig, provider); err != nil {
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

func newProviderIdpContext(idp *gatewayv1.ExternalIdentityProvider, scopes []string) idpContext {
	return idpContext{
		client:       idp.Client,
		basic:        idp.Basic,
		scopes:       scopes,
		grantType:    idp.GrantType,
		tokenRequest: idp.TokenRequest,
	}
}

// newConsumerIdpContext uses the consumer credentials and scopes. The grant type and token request method
// fall back to the provider settings unless the consumer overrides them.
func newConsumerIdpContext(m2m *gatewayv1.ConsumerMachine2MachineAuthentication, idp *gatewayv1.ExternalIdentityProvider) idpContext {
	idpCtx := idpContext{
		client:       m2m.Client,
		basic:        m2m.Basic,
		scopes:       m2m.Scopes,
		grantType:    idp.GrantType,
		tokenRequest: idp.TokenRequest,
	}
	if m2m.GrantType != "" {
		idpCtx.grantType = m2m.GrantType
	}
	if m2m.TokenRequest != "" {
		idpCtx.tokenRequest = m2m.TokenRequest
	}
	return idpCtx
}

// hasCredentialsForGrant reports whether the credentials can build a token request for the grant type.
// Jumper sends every configured credential together with the grant type and does not check if they match.
func hasCredentialsForGrant(idpCtx idpContext) (bool, error) {
	switch idpCtx.grantType {
	case gatewayv1.GrantTypePassword:
		return idpCtx.basic != nil, nil
	case gatewayv1.GrantTypeRefreshToken:
		return idpCtx.client != nil && idpCtx.client.RefreshToken != "", nil
	case gatewayv1.GrantTypeAuthorizationCode, gatewayv1.GrantTypeClientCredentials:
		return idpCtx.client != nil, nil
	default:
		return false, fmt.Errorf("unknown grantType: %s", idpCtx.grantType)
	}
}

// applyCredentials publishes the credentials used for the grant type. Username/password are only used with the
// password grant, client credentials authenticate the client for every grant type.
func applyCredentials(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, idpCtx idpContext) error {
	if idpCtx.client != nil {
		if err := applyOauth(ctx, key, jumperConfig, idpCtx); err != nil {
			return err
		}
	}
	if idpCtx.grantType == gatewayv1.GrantTypePassword {
		return applyBasic(ctx, key, jumperConfig, idpCtx)
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
	idpCtx := newConsumerIdpContext(consumerM2M, providerSettings)

	if ok, err := hasCredentialsForGrant(idpCtx); err != nil {
		return err
	} else if ok {
		return applyCredentials(ctx, key, jumperConfig, idpCtx)
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

func applyOauth(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, idpCtx idpContext) error {
	current := jumperConfig.OAuth[key]
	oauth, err := extendOauth(ctx, &current, idpCtx)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = oauth

	return nil
}

func extendOauth(ctx context.Context, current *plugin.OauthCredentials, idpCtx idpContext) (plugin.OauthCredentials, error) {
	in := *current
	var err error

	client := idpCtx.client
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

	if len(idpCtx.scopes) > 0 {
		in.Scopes = strings.Join(idpCtx.scopes, " ")
	}

	tokenRequest, err := tokenRequestToJumper(idpCtx.tokenRequest)
	if err != nil {
		return in, err
	}
	in.TokenRequest = tokenRequest
	in.GrantType = string(idpCtx.grantType)

	return in, nil
}

func applyBasic(ctx context.Context, key plugin.ConsumerId, jumperConfig *plugin.JumperConfig, idpCtx idpContext) error {
	current := jumperConfig.OAuth[key]
	basicAuth, err := extendBasic(ctx, &current, idpCtx)
	if err != nil {
		return err
	}
	jumperConfig.OAuth[key] = basicAuth
	return nil
}

func extendBasic(ctx context.Context, current *plugin.OauthCredentials, idpCtx idpContext) (plugin.OauthCredentials, error) {
	in := *current
	var err error

	in.Username = idpCtx.basic.Username
	password := idpCtx.basic.Password
	if password != "" {
		password, err = secretManagerApi.Get(ctx, password)
		if err != nil {
			return in, err
		}
	}

	if len(idpCtx.scopes) > 0 {
		in.Scopes = strings.Join(idpCtx.scopes, " ")
	}

	in.Password = password
	in.GrantType = string(idpCtx.grantType)

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
