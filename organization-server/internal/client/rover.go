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
)

// RoverClient calls rover-server endpoints, passing through the consumer's
// original token for prefix-based scoping.
type RoverClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRoverClient creates a new rover-server client.
func NewRoverClient(baseURL string) *RoverClient {
	return &RoverClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
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

// GetResources calls GET /resources on rover-server with optional prefix filter.
// The consumerToken is passed through directly for scoping.
func (r *RoverClient) GetResources(ctx context.Context, consumerToken string, prefix string) (*ResourceListResponse, error) {
	url := r.baseURL + "/resources"
	if prefix != "" {
		url += "?prefix=" + prefix
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	if consumerToken != "" {
		req.Header.Set("Authorization", "Bearer "+consumerToken)
	}

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
