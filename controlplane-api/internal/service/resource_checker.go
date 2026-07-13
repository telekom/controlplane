// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

// ResourceChecker checks whether a team has resources via rover-server.
type ResourceChecker interface {
	// HasResources returns true if the team identified by the given prefix
	// has any resources (Rovers, ApiSpecs, EventSpecs, etc.).
	HasResources(ctx context.Context, prefix string) (bool, error)
}

// roverResourceChecker implements ResourceChecker by calling rover-server's
// GET /resources?prefix=<prefix> endpoint.
type roverResourceChecker struct {
	baseURL     string
	environment string
	scopePrefix string
	httpClient  *http.Client
}

// NewRoverResourceChecker creates a ResourceChecker that calls rover-server.
// baseURL should be the rover-server internal URL (e.g. http://rover-server.controlplane-system.svc.cluster.local).
// environment is the environment claim for the mock token (e.g. "poc").
// scopePrefix is the scope prefix rover-server expects (e.g. "tardis").
func NewRoverResourceChecker(baseURL, environment, scopePrefix string) ResourceChecker {
	return &roverResourceChecker{
		baseURL:     baseURL,
		environment: environment,
		scopePrefix: scopePrefix,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (r *roverResourceChecker) HasResources(ctx context.Context, prefix string) (bool, error) {
	url := fmt.Sprintf("%s/resources?prefix=%s&limit=1", r.baseURL, prefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("building request: %w", err)
	}

	token := mock.NewMockAccessToken(r.environment, "cpapi", "service", []string{r.scopePrefix + ":admin:all"})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("calling rover-server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("rover-server returned status %d", resp.StatusCode)
	}

	var result struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decoding rover-server response: %w", err)
	}

	return len(result.Items) > 0, nil
}

// noopResourceChecker always returns false (no resources). Used when
// rover-server integration is disabled.
type noopResourceChecker struct{}

func NewNoopResourceChecker() ResourceChecker {
	return &noopResourceChecker{}
}

func (n *noopResourceChecker) HasResources(_ context.Context, _ string) (bool, error) {
	return false, nil
}
