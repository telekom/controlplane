// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	client "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"golang.org/x/oauth2"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

// ClientFactory creates RealmClient instances for a given RealmStatus.
type ClientFactory interface {
	ClientFor(realmStatus identityv1.RealmStatus) (RealmClient, error)
}

// clientFactory is the default production implementation of ClientFactory.
type clientFactory struct{}

// NewClientFactory creates a new production ClientFactory.
func NewClientFactory() ClientFactory {
	return &clientFactory{}
}

func (f *clientFactory) ClientFor(realmStatus identityv1.RealmStatus) (RealmClient, error) {
	return GetClientForRealm(realmStatus)
}

// ClientFactoryFunc adapts a plain function to the ClientFactory interface.
// Useful for tests.
type ClientFactoryFunc func(realmStatus identityv1.RealmStatus) (RealmClient, error)

func (f ClientFactoryFunc) ClientFor(realmStatus identityv1.RealmStatus) (RealmClient, error) {
	return f(realmStatus)
}

type AdminConfig interface {
	EndpointUrl() string
	IssuerUrl() string
	TokenUrl() string
	ClientId() string
	ClientSecret() string
	Username() string
	Password() string
}

type adminConfig struct {
	endpointUrl  string
	issuerUrl    string
	tokenUrl     string
	clientId     string
	clientSecret string
	username     string
	password     string
}

func (a adminConfig) EndpointUrl() string {
	return a.endpointUrl
}

func (a adminConfig) IssuerUrl() string {
	return a.issuerUrl
}

func (a adminConfig) TokenUrl() string {
	return a.tokenUrl
}

func (a adminConfig) ClientId() string {
	return a.clientId
}

func (a adminConfig) ClientSecret() string {
	return a.clientSecret
}

func (a adminConfig) Username() string {
	return a.username
}

func (a adminConfig) Password() string {
	return a.password
}

func NewKeycloakClientConfig(
	endpointUrl, issuerUrl, tokenUrl, clientId, clientSecret, username, password string) AdminConfig {
	return &adminConfig{
		endpointUrl:  endpointUrl,
		issuerUrl:    issuerUrl,
		tokenUrl:     tokenUrl,
		clientId:     clientId,
		clientSecret: clientSecret,
		username:     username,
		password:     password,
	}
}

func newOauth2Client(config AdminConfig) (*http.Client, error) {
	// Create a base client with TLS configuration from common-server
	baseClient := client.NewBaseHttpClient(
		client.WithClientTimeout(10 * time.Second),
	)

	tokenUrl, err := url.Parse(config.TokenUrl())
	if err != nil {
		return nil, fmt.Errorf("failed to parse token URL: %w", err)
	}

	// Add resource owner password credentials to the token configuration
	credentialsCfg := oauth2.Config{
		ClientID:     config.ClientId(),
		ClientSecret: config.ClientSecret(),
		Endpoint: oauth2.Endpoint{
			TokenURL: tokenUrl.String(),
		},
	}

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, baseClient)
	token, err := credentialsCfg.PasswordCredentialsToken(ctx, config.Username(), config.Password())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	tokenSource := credentialsCfg.TokenSource(ctx, token)
	httpClient := oauth2.NewClient(ctx, tokenSource)
	return httpClient, nil
}

var NewClientFor = func(config AdminConfig) (*api.ClientWithResponses, error) {
	// create a client with sane default values
	oauth2Client, err := newOauth2Client(config)
	if err != nil {
		return nil, err
	}

	endpointUrl, err := url.Parse(config.EndpointUrl())
	if err != nil {
		return nil, fmt.Errorf("failed to parse keycloak admin URL: %w", err)
	}

	// Add metrics wrapper around the HTTP client
	metricsClient := metrics.WithMetrics(oauth2Client,
		metrics.WithClientName("identity"),
		metrics.WithReplacePatterns(`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID),
	)

	apiClient, err := api.NewClientWithResponses(endpointUrl.String(), api.WithHTTPClient(metricsClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create keycloak admin client: %w", err)
	}

	return apiClient, nil
}

const EmptyString = ""

func GetClientForRealm(realmStatus identityv1.RealmStatus) (RealmClient, error) {
	keycloakClientConfig := NewKeycloakClientConfig(
		realmStatus.AdminUrl,
		realmStatus.IssuerUrl,
		realmStatus.AdminTokenUrl,
		realmStatus.AdminClientId,
		EmptyString, // client secret is not needed for the admin client, because PasswordCredentialsFlow must be used
		realmStatus.AdminUserName,
		realmStatus.AdminPassword,
	)

	clientWithResponses, err := NewClientFor(keycloakClientConfig)
	if err != nil {
		return nil, err
	} else {
		client := NewRealmClient(clientWithResponses)
		return client, nil
	}
}

var GetClientFor = func(realmStatus identityv1.RealmStatus) (RealmClient, error) {
	return GetClientForRealm(realmStatus)
}
