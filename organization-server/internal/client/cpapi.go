// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CPAPIClient calls the controlplane-api GraphQL endpoint.
// It authenticates as admin using the TokenSource and forwards the original
// consumer identity via X-Forwarded-* headers.
type CPAPIClient struct {
	endpoint    string
	tokenSource *TokenSource
	httpClient  *http.Client
}

// NewCPAPIClient creates a new CP API client.
func NewCPAPIClient(endpoint string, tokenSource *TokenSource) *CPAPIClient {
	return &CPAPIClient{
		endpoint:    endpoint,
		tokenSource: tokenSource,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// GraphQLRequest is a standard GraphQL request body.
type GraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// GraphQLResponse is a standard GraphQL response body.
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error.
type GraphQLError struct {
	Message string `json:"message"`
	Path    []any  `json:"path,omitempty"`
}

// ConsumerIdentity holds the forwarded consumer context.
type ConsumerIdentity struct {
	Environment string
	Group       string
	Team        string
}

// Execute performs a GraphQL query against CP API, authenticating as admin
// and forwarding the consumer identity via X-Forwarded headers.
func (c *CPAPIClient) Execute(ctx context.Context, req GraphQLRequest, identity *ConsumerIdentity) (*GraphQLResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling graphql request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Authenticate as admin via facade's own token.
	if c.tokenSource != nil {
		token, err := c.tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("getting admin token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	// Forward consumer identity so CP API builds the correct Viewer.
	if identity != nil {
		httpReq.Header.Set("X-Forwarded-Environment", identity.Environment)
		if identity.Group != "" {
			httpReq.Header.Set("X-Forwarded-Group", identity.Group)
		}
		if identity.Team != "" {
			httpReq.Header.Set("X-Forwarded-Team", identity.Team)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling CP API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CP API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("decoding graphql response: %w", err)
	}

	return &gqlResp, nil
}
