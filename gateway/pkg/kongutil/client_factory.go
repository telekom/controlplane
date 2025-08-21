// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"
	metrics "github.com/telekom/controlplane/common-server/pkg/client/metrics"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type GatewayAdminConfig interface {
	AdminUrl() string
	AdminClientId() string
	AdminClientSecret() string
	AdminIssuer() string
}

type gatewayAdminConfig struct {
	url          string
	clientId     string
	clientSecret string
	issuer       string
}

func (g *gatewayAdminConfig) AdminUrl() string {
	return g.url
}

func (g *gatewayAdminConfig) AdminClientId() string {
	return g.clientId
}

func (g *gatewayAdminConfig) AdminClientSecret() string {
	return g.clientSecret
}

func (g *gatewayAdminConfig) AdminIssuer() string {
	return g.issuer
}

func NewGatewayConfig(rawUrl string, clientId, clientSecret, issuer string) GatewayAdminConfig {
	return &gatewayAdminConfig{
		url:          rawUrl,
		clientId:     clientId,
		clientSecret: clientSecret,
		issuer:       issuer,
	}
}

var (
	rootCtx      = context.Background()
	tokenUrlPath = "/protocol/openid-connect/token"

	clientCache      = make(map[string]client.KongClient)
	clientCacheMutex sync.Mutex
)

var GetClientFor = func(gwCfg GatewayAdminConfig) (client.KongClient, error) {
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()
	if client, ok := clientCache[gwCfg.AdminUrl()]; ok {
		return client, nil
	}
	apiClient, err := NewClientFor(gwCfg)
	if err != nil {
		return nil, err
	}
	c := client.NewKongClient(apiClient)
	clientCache[gwCfg.AdminUrl()] = c
	return c, err
}

var NewClientFor = func(gwCfg GatewayAdminConfig) (kong.ClientWithResponsesInterface, error) {
	baseClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 10 * time.Second,
	}

	tokenCfg := clientcredentials.Config{
		ClientID:     gwCfg.AdminClientId(),
		ClientSecret: gwCfg.AdminClientSecret(),
		TokenURL:     gwCfg.AdminIssuer() + tokenUrlPath,
	}

	ctx := context.WithValue(rootCtx, oauth2.HTTPClient, baseClient)

	url, err := url.Parse(gwCfg.AdminUrl())
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse gateway URL")
	}

	httpClient := tokenCfg.Client(ctx)
	metricsClient := metrics.WithMetrics(httpClient,
		metrics.WithClientName("gateway"),
		metrics.WithReplacePatterns(`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID),
	)

	apiClient, err := kong.NewClientWithResponses(url.String(), kong.WithHTTPClient(metricsClient))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kong client")
	}

	return apiClient, nil
}
