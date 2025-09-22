// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"golang.org/x/oauth2"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

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
	// Create a client with sane default values
	baseClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 10 * time.Second,
	}

	tokenUrl, err := url.Parse(config.TokenUrl())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse token URL")
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
		return nil, errors.Wrap(err, "failed to retrieve token")
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
		return nil, errors.Wrap(err, "failed to parse keycloak admin URL")
	}

	// Add metrics wrapper around the HTTP client
	metricsClient := metrics.WithMetrics(oauth2Client,
		metrics.WithClientName("identity"),
		metrics.WithReplacePatterns(`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID),
	)

	apiClient, err := api.NewClientWithResponses(endpointUrl.String(), api.WithHTTPClient(metricsClient))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create keycloak admin client")
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
