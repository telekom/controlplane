// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	metrics "github.com/telekom/controlplane/common-server/pkg/client/metrics"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
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

	migrator legacy.Migrator = legacy.None()
)

// SetMigrator sets the legacy Migrator injected into every Kong client created afterwards
// and drops the cached clients. It is called once at startup.
func SetMigrator(m legacy.Migrator) {
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()
	if m == nil {
		m = legacy.None()
	}
	migrator = m
	clientCache = make(map[string]client.KongClient)
	urlToKey = make(map[string]string)
}

// cacheKey produces a SHA-256 hex digest of all connection parameters.
// When any parameter (including the secret) changes, the key changes,
// causing an automatic cache miss and fresh client creation.
func cacheKey(gwCfg GatewayAdminConfig) string {
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%s",
		gwCfg.AdminUrl(), gwCfg.AdminClientId(),
		gwCfg.AdminClientSecret(), gwCfg.AdminIssuer())
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
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
	c := client.NewKongClient(apiClient, client.WithMigrator(migrator))

	// Evict stale entry for the same URL (previous credentials).
	adminUrl := gwCfg.AdminUrl()
	if oldKey, ok := urlToKey[adminUrl]; ok && oldKey != key {
		delete(clientCache, oldKey)
	}
	urlToKey[adminUrl] = key
	clientCache[key] = c

	return c, nil
}

// EnvSkipTlsVerify is the environment variable that disables TLS
// certificate and hostname verification for Kong Admin API and OAuth
// token requests. Only the value "true" (case-insensitive) enables it.
const EnvSkipTlsVerify = "SKIP_TLS_VERIFY"

func skipTlsVerify() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvSkipTlsVerify)), "true")
}

var NewClientFor = func(gwCfg GatewayAdminConfig) (kong.ClientWithResponsesInterface, error) {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
	}
	if skipTlsVerify() {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via SKIP_TLS_VERIFY
	}
	baseClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	tokenUrl, err := url.JoinPath(gwCfg.AdminIssuer(), tokenUrlPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to construct token URL")
	}

	tokenCfg := clientcredentials.Config{
		ClientID:     gwCfg.AdminClientId(),
		ClientSecret: gwCfg.AdminClientSecret(),
		TokenURL:     tokenUrl,
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
