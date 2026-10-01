// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type oidcMetadata struct {
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
}

const maxOIDCMetadataSize = 1 << 20

func discoverTokenURL(ctx context.Context, client *http.Client, issuerURL string) (tokenURL string, err error) {
	if client == nil {
		return "", fmt.Errorf("OIDC discovery HTTP client is nil")
	}
	discoveryURL := strings.TrimSuffix(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("creating OIDC discovery request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting OIDC metadata: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOIDCMetadataSize+1))
	if err != nil {
		return "", fmt.Errorf("reading OIDC metadata: %w", err)
	}
	if len(body) > maxOIDCMetadataSize {
		return "", fmt.Errorf("OIDC metadata exceeds %d bytes", maxOIDCMetadataSize)
	}
	var metadata oidcMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return "", fmt.Errorf("decoding OIDC metadata: %w", err)
	}
	if strings.TrimSuffix(metadata.Issuer, "/") != strings.TrimSuffix(issuerURL, "/") {
		return "", fmt.Errorf("OIDC issuer mismatch: got %q, want %q", metadata.Issuer, issuerURL)
	}
	if err := validateDiscoveredTokenURL(metadata.TokenEndpoint); err != nil {
		return "", err
	}
	return metadata.TokenEndpoint, nil
}

func validateDiscoveredTokenURL(rawURL string) error {
	tokenURL, err := url.ParseRequestURI(rawURL)
	if err != nil || !tokenURL.IsAbs() || tokenURL.Scheme != "https" || tokenURL.Hostname() == "" || tokenURL.User != nil || tokenURL.Fragment != "" {
		return fmt.Errorf("OIDC token endpoint must be an absolute HTTPS URL without userinfo or fragment")
	}
	if tokenURL.RawQuery != "" {
		return fmt.Errorf("OIDC token endpoint must not contain a query")
	}
	return nil
}
