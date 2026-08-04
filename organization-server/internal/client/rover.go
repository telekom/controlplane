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
	"net/url"
	"strings"
	"time"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

const maxRoverResponseBytes = 4 * 1024 * 1024

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
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
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
	Links struct {
		Next string `json:"next"`
	} `json:"_links"`
	Items []ResourceRef `json:"items"`
}

// GetResources calls GET /resources on rover-server for a specific team.
// Authentication is the projected SA token; rover-server's internal listener
// derives its admin business context from the X-Environment header.
func (r *RoverClient) GetResources(ctx context.Context, environment, group, team string) (*ResourceListResponse, error) {
	baseURL, err := url.Parse(r.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing rover-server base URL: %w", err)
	}
	if baseURL.User != nil {
		return nil, fmt.Errorf("refusing rover-server base URL with userinfo")
	}
	requestURL := *baseURL
	requestURL.Path = strings.TrimSuffix(requestURL.Path, "/") + "/resources"
	query := requestURL.Query()
	query.Set("group", group)
	query.Set("team", team)
	requestURL.RawQuery = query.Encode()

	result := &ResourceListResponse{}
	pageURL := &requestURL
	visited := make(map[string]struct{})
	for pages := 0; pageURL != nil; pages++ {
		if pages == 1000 {
			return nil, fmt.Errorf("following rover-server pagination: page limit of 1000 exceeded")
		}
		pageURLString := pageURL.String()
		if _, ok := visited[pageURLString]; ok {
			return nil, fmt.Errorf("rover-server pagination loop at %q", pageURLString)
		}
		visited[pageURLString] = struct{}{}
		if pageURL.User != nil {
			return nil, fmt.Errorf("refusing rover-server page URL with userinfo")
		}
		if pageURL.Scheme != baseURL.Scheme || pageURL.Host != baseURL.Host {
			return nil, fmt.Errorf("refusing rover-server page URL for different origin")
		}

		page, err := r.getResourcePage(ctx, pageURLString, environment)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, page.Items...)
		if page.Links.Next == "" {
			pageURL = nil
			continue
		}
		nextURL, err := url.Parse(page.Links.Next)
		if err != nil {
			return nil, fmt.Errorf("parsing rover-server page URL: %w", err)
		}
		pageURL = pageURL.ResolveReference(nextURL)
	}

	return result, nil
}

func (r *RoverClient) getResourcePage(ctx context.Context, pageURL, environment string) (*ResourceListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, http.NoBody)
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRoverResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if len(body) > maxRoverResponseBytes {
		return nil, fmt.Errorf("rover-server response exceeds %d bytes", maxRoverResponseBytes)
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
