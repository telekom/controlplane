// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	metrics "github.com/telekom/controlplane/common-server/pkg/client/metrics"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
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

func NewGatewayConfig(rawUrl, clientId, clientSecret, issuer string) GatewayAdminConfig {
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
	urlToKey         = make(map[string]string) // AdminUrl -> current cache key for stale eviction
	clientCacheMutex sync.Mutex
)

// cacheKey produces a SHA-256 hex digest of all connection parameters.
// When any parameter (including the secret) changes, the key changes,
// causing an automatic cache miss and fresh client creation.
func cacheKey(gwCfg GatewayAdminConfig) string {
	h := sha256.New()
	for i, value := range []string{
		gwCfg.AdminUrl(), gwCfg.AdminClientId(), gwCfg.AdminClientSecret(), gwCfg.AdminIssuer(),
	} {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(value))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

var GetClientFor = func(gwCfg GatewayAdminConfig) (client.KongClient, error) {
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()

	key := cacheKey(gwCfg)

	if c, ok := clientCache[key]; ok {
		return c, nil
	}

	apiClient, err := NewClientFor(gwCfg)
	if err != nil {
		return nil, err
	}
	c := client.NewKongClient(apiClient)

	// Evict stale entry for the same URL (previous credentials).
	adminUrl := gwCfg.AdminUrl()
	if oldKey, ok := urlToKey[adminUrl]; ok && oldKey != key {
		delete(clientCache, oldKey)
	}
	urlToKey[adminUrl] = key
	clientCache[key] = c

	return c, nil
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
