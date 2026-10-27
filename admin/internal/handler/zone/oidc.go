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
	"time"

	"github.com/telekom/controlplane/common-server/pkg/client/tlsclient"
)

type oidcMetadata struct {
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
}

const maxOIDCMetadataSize = 1 << 20

// ReasonOIDCIssuerMismatch is the Zone event reason for a discovered issuer that differs from the configured one.
const ReasonOIDCIssuerMismatch = "OIDCIssuerMismatch"

// discoveryProfile matches the former default discovery client: default
// transport (proxy from environment) and a 10s timeout.
var discoveryProfile = tlsclient.Profile{
	Name:      "admin-oidc-discovery",
	Transport: tlsclient.TransportDefault,
	Timeout:   10 * time.Second,
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// staticCA serves an already validated canonical bundle.
type staticCA []byte

func (s staticCA) Resolve(context.Context) ([]byte, error) {
	return s, nil
}

// discoveryClient returns the doer for OIDC discovery of issuerURL and a func
// releasing it. HandlingContext.HTTPClient takes precedence; otherwise a
// standalone client trusts the system roots and the zone CA bundle.
func discoveryClient(ctx context.Context, hc *HandlingContext, issuerURL string) (httpDoer, func(), error) {
	if hc.HTTPClient != nil {
		return hc.HTTPClient, func() {}, nil
	}
	var ca tlsclient.CAResolver
	if hc.CABundle != nil {
		ca = staticCA(hc.CABundle)
	}
	c, err := tlsclient.New(ctx, discoveryProfile, tlsclient.Spec{
		APIURL: issuerURL,
		Auth:   tlsclient.Auth{Mode: tlsclient.AuthNone},
		CA:     ca,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating OIDC discovery client: %w", err)
	}
	return c.Doer(), c.Close, nil
}

// discoverTokenURL returns the validated token endpoint and the issuer advertised in the OIDC metadata.
func discoverTokenURL(ctx context.Context, client httpDoer, issuerURL string) (tokenURL, advertisedIssuer string, err error) {
	if client == nil {
		return "", "", fmt.Errorf("OIDC discovery HTTP client is nil")
	}
	discoveryURL := strings.TrimSuffix(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("creating OIDC discovery request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("requesting OIDC metadata: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOIDCMetadataSize+1))
	if err != nil {
		return "", "", fmt.Errorf("reading OIDC metadata: %w", err)
	}
	if len(body) > maxOIDCMetadataSize {
		return "", "", fmt.Errorf("OIDC metadata exceeds %d bytes", maxOIDCMetadataSize)
	}
	var metadata oidcMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return "", "", fmt.Errorf("decoding OIDC metadata: %w", err)
	}
	if err := validateDiscoveredTokenURL(metadata.TokenEndpoint); err != nil {
		return "", "", err
	}
	return metadata.TokenEndpoint, metadata.Issuer, nil
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

func issuersEqual(a, b string) bool {
	return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/")
}
