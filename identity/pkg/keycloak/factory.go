// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"sync"
	"time"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"golang.org/x/oauth2"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

// ServiceFactory creates KeycloakService instances for a given RealmStatus.
type ServiceFactory interface {
	ServiceFor(realmStatus identityv1.RealmStatus) (KeycloakService, error)
}

// ServiceFactoryFunc adapts a plain function to the
// ServiceFactory interface. Useful for tests.
type ServiceFactoryFunc func(identityv1.RealmStatus) (KeycloakService, error)

func (f ServiceFactoryFunc) ServiceFor(s identityv1.RealmStatus) (KeycloakService, error) {
	return f(s)
}

// serviceFactory is the production implementation with caching.
// Cached services are keyed by a SHA-256 digest of the connection
// parameters so that credentials are not stored as plaintext map keys.
type serviceFactory struct {
	mu    sync.RWMutex
	cache map[string]KeycloakService
}

// NewServiceFactory creates a new cached production factory.
func NewServiceFactory() ServiceFactory {
	return &serviceFactory{cache: make(map[string]KeycloakService)}
}

func (f *serviceFactory) ServiceFor(status identityv1.RealmStatus) (KeycloakService, error) {
	key := cacheKey(status)

	// Fast path: read lock only.
	f.mu.RLock()
	if svc, ok := f.cache[key]; ok {
		f.mu.RUnlock()
		return svc, nil
	}
	f.mu.RUnlock()

	// Slow path: acquire write lock, double-check.
	f.mu.Lock()
	defer f.mu.Unlock()

	if svc, ok := f.cache[key]; ok {
		return svc, nil
	}

	svc, err := NewKeycloakServiceFor(status)
	if err != nil {
		return nil, err
	}

	f.cache[key] = svc
	return svc, nil
}

// cacheKey produces a SHA-256 hex digest of the connection parameters.
func cacheKey(s identityv1.RealmStatus) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s",
		s.AdminUrl, s.AdminTokenUrl, s.AdminClientId,
		s.AdminUserName, s.AdminPassword)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// NewKeycloakServiceFor builds the full HTTP client stack and wraps
// it in a KeycloakService.
func NewKeycloakServiceFor(status identityv1.RealmStatus) (KeycloakService, error) {
	tokenUrl, err := url.Parse(status.AdminTokenUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token URL: %w", err)
	}

	endpointUrl, err := url.Parse(status.AdminUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse keycloak admin URL: %w", err)
	}

	// 1. Base HTTP client with TLS + timeout.
	baseClient := commonclient.NewBaseHttpClient(
		commonclient.WithClientTimeout(10 * time.Second),
	)

	// 2. OAuth2 token via Resource Owner Password Credentials.
	oauth2Cfg := oauth2.Config{
		ClientID: status.AdminClientId,
		Endpoint: oauth2.Endpoint{TokenURL: tokenUrl.String()},
	}

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, baseClient)
	token, err := oauth2Cfg.PasswordCredentialsToken(ctx, status.AdminUserName, status.AdminPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	httpClient := oauth2.NewClient(ctx, newReAuthTokenSource(ctx, &oauth2Cfg, status.AdminUserName, status.AdminPassword, token))

	// 3. Metrics wrapper.
	metricsClient := metrics.WithMetrics(httpClient,
		metrics.WithClientName("identity"),
		metrics.WithReplacePatterns(
			`([^\/]+--[^\/]+--[^\/]+)`,
			`([^\/]+--[^\/]+)`,
			metrics.ReplacePatternUID,
		),
	)

	// 4. oapi-codegen typed client.
	apiClient, err := api.NewClientWithResponses(endpointUrl.String(), api.WithHTTPClient(metricsClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create keycloak admin client: %w", err)
	}

	// 5. Domain service.
	return NewKeycloakService(apiClient), nil
}
