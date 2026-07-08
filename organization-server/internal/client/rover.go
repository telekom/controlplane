// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

// RoverClient calls rover-server endpoints using service-level mock tokens.
// It constructs the correct prefix and token from the caller's identity,
// rather than forwarding external tokens.
type RoverClient struct {
	baseURL     string
	environment string
	scopePrefix string
	httpClient  *http.Client
}

// NewRoverClient creates a new rover-server client.
// environment is the env claim for mock tokens (e.g. "controlplane").
// scopePrefix is the scope prefix rover-server expects (e.g. "tardis").
func NewRoverClient(baseURL, environment, scopePrefix string) *RoverClient {
	return &RoverClient{
		baseURL:     baseURL,
		environment: environment,
		scopePrefix: scopePrefix,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// ResourceRef matches the rover-server ResourceRef schema.
type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Path       string `json:"path"`
}

// ResourceListResponse matches the rover-server response shape.
type ResourceListResponse struct {
	Items []ResourceRef `json:"items"`
}

// GetResources calls GET /resources on rover-server for a specific team.
// It constructs a mock admin token and the appropriate prefix from the team identity.
func (r *RoverClient) GetResources(ctx context.Context, group, team string) (*ResourceListResponse, error) {
	prefix := fmt.Sprintf("%s--%s--%s/", r.environment, group, team)
	url := fmt.Sprintf("%s/resources?prefix=%s", r.baseURL, prefix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	token := mock.NewMockAccessToken(r.environment, "org-server", "service", []string{r.scopePrefix + ":admin:all"})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling rover-server: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rover-server returned %d: %s", resp.StatusCode, string(body))
	}

	var result ResourceListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}
