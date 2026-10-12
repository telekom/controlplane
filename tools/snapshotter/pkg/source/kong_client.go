// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// NewKongClient creates a standalone Kong client using OAuth client credentials.
func NewKongClient(cfg config.SourceConfig) (kong.ClientWithResponsesInterface, error) {
	adminURL, err := url.Parse(cfg.AdminUrl())
	if err != nil {
		return nil, fmt.Errorf("failed to parse gateway URL: %w", err)
	}

	transport := &http.Transport{MaxIdleConnsPerHost: 100}
	if cfg.InsecureSkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // Explicit per-source opt-in for test environments.
		}
	}
	// OAuth token requests and authenticated API requests share this transport.
	baseClient := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	tokenConfig := clientcredentials.Config{
		ClientID:     cfg.AdminClientId(),
		ClientSecret: cfg.AdminClientSecret(),
		TokenURL:     cfg.AdminIssuer() + "/protocol/openid-connect/token",
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, baseClient)
	httpClient := tokenConfig.Client(ctx)
	metricsClient := metrics.WithMetrics(httpClient,
		metrics.WithClientName("gateway"),
		metrics.WithReplacePatterns(`([^\/]+--[^\/]+--[^\/]+)`, `([^\/]+--[^\/]+)`, metrics.ReplacePatternUID),
	)
	apiClient, err := kong.NewClientWithResponses(adminURL.String(), kong.WithHTTPClient(metricsClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create kong client: %w", err)
	}
	return apiClient, nil
}
