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

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

// RoverClient calls rover-server's internal (Kubernetes-authz) listener using
// the projected ServiceAccount token for rover-server's audience. It never
// forwards the external caller's token; the consumer identity travels as
// explicit query params plus the X-Environment header.
type RoverClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRoverClient creates a new rover-server client.
// tokenFilePath is the projected ServiceAccount token for rover-server's
// audience; caFilePath is the CA bundle verifying rover-server's TLS cert.
func NewRoverClient(baseURL string, token accesstoken.AccessToken, caFilePath string) *RoverClient {
	httpClient := commonclient.NewBaseHttpClient(
		commonclient.WithCaFilepath(caFilePath),
		commonclient.WithClientName("rover"),
		commonclient.WithClientTimeout(10*time.Second),
	)
	httpClient.Transport = &tokenTransport{
		token: token,
		base:  httpClient.Transport,
	}
	return &RoverClient{
		baseURL:    baseURL,
		httpClient: httpClient,
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
// Authentication is the projected SA token; rover-server's internal listener
// derives its admin business context from the X-Environment header.
func (r *RoverClient) GetResources(ctx context.Context, environment, group, team string) (*ResourceListResponse, error) {
	url := fmt.Sprintf("%s/resources?group=%s&team=%s", r.baseURL, group, team)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("X-Environment", environment)
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling rover-server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort close

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
