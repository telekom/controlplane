// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

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

// FrontendUrlAttribute is the Keycloak realm attribute that overrides the
// public base URL (and therefore the issuer) of a realm.
const FrontendUrlAttribute = "frontendUrl"

const (
	oidcDiscoverySuffix = "/.well-known/openid-configuration"
	// maxDiscoveryBodySize bounds the OIDC discovery response that is read.
	maxDiscoveryBodySize = 1 << 20
)

// ValidateRealmAttributes validates realm attributes before they are applied.
// Keys must not be empty. Values, including frontendUrl, are not validated;
// they are passed to Keycloak unchanged.
func ValidateRealmAttributes(attributes map[string]string) error {
	for key := range attributes {
		if key == "" {
			return fmt.Errorf("realm attribute keys must not be empty")
		}
	}
	return nil
}

// ValidateFrontendUrl checks that the value is an absolute http(s) base URL
// with a host and without credentials, query or fragment. Context paths such
// as "/auth" and trailing slashes are allowed.
func ValidateFrontendUrl(frontendUrl string) error {
	_, err := parseBaseUrl(frontendUrl)
	return err
}

// IssuerUrlFromFrontendUrl returns the issuer Keycloak advertises for a realm
// whose frontend URL is set: "<frontendUrl>/realms/<realm>". Like Keycloak, a
// trailing slash of the frontend URL is ignored and default ports are dropped.
func IssuerUrlFromFrontendUrl(frontendUrl, realmName string) (string, error) {
	u, err := parseBaseUrl(frontendUrl)
	if err != nil {
		return "", err
	}
	host := u.Host
	if (strings.EqualFold(u.Scheme, "https") && u.Port() == "443") || (strings.EqualFold(u.Scheme, "http") && u.Port() == "80") {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	return u.Scheme + "://" + host + path + "/realms/" + url.PathEscape(realmName), nil
}

func parseBaseUrl(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("URL must not contain surrounding whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	return u, validateBaseUrl(u, raw)
}

func validateBaseUrl(u *url.URL, raw string) error {
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("URL must use the http or https scheme")
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return fmt.Errorf("URL must be absolute and contain a host")
	}
	if u.User != nil {
		return fmt.Errorf("URL must not contain credentials")
	}
	if u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "?") {
		return fmt.Errorf("URL must not contain a query")
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return fmt.Errorf("URL must not contain a fragment")
	}
	return nil
}

// validateDiscoveredIssuer checks that the issuer advertised by Keycloak is a
// safe absolute http(s) URL ending with "/realms/<realm>".
func validateDiscoveredIssuer(issuer, realmName string) error {
	u, err := url.Parse(issuer)
	if err != nil {
		return fmt.Errorf("invalid issuer URL: %w", err)
	}
	if err := validateBaseUrl(u, issuer); err != nil {
		return fmt.Errorf("invalid issuer URL: %w", err)
	}
	if !strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/realms/"+realmName) {
		return fmt.Errorf("issuer URL does not belong to realm %q", realmName)
	}
	return nil
}

// issuerDiscovery fetches the issuer advertised by Keycloak's OIDC discovery
// document. It must use an HTTP client without admin credentials.
type issuerDiscovery struct {
	httpClient *http.Client
	adminUrl   string
}

// discoveryUrl returns the discovery document URL of a realm, reached via the
// management (admin) URL. This only selects the transport address; the
// issuer itself is read from the document.
func (d *issuerDiscovery) discoveryUrl(realmName string) string {
	return DetermineIssuerUrlFrom(d.adminUrl, url.PathEscape(realmName)) + oidcDiscoverySuffix
}

func (d *issuerDiscovery) discoverIssuer(ctx context.Context, realmName string) (issuer string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.discoveryUrl(realmName), http.NoBody)
	if err != nil {
		return "", blockedErrorf("creating OIDC discovery request: %s", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", retryableErrorf("requesting OIDC discovery document: %s", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing OIDC discovery response: %w", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", retryableErrorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBodySize+1))
	if err != nil {
		return "", retryableErrorf("reading OIDC discovery document: %s", err)
	}
	if len(body) > maxDiscoveryBodySize {
		return "", blockedErrorf("OIDC discovery document exceeds %d bytes", maxDiscoveryBodySize)
	}

	var metadata struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return "", blockedErrorf("decoding OIDC discovery document: %s", err)
	}
	if err := validateDiscoveredIssuer(metadata.Issuer, realmName); err != nil {
		return "", blockedErrorf("OIDC discovery document: %s", err)
	}
	return metadata.Issuer, nil
}

// blockedErrorf returns an ApiError that is classified as blocked.
func blockedErrorf(format string, a ...any) ApiError {
	return &apiError{message: fmt.Sprintf(format, a...)}
}

// retryableErrorf returns an ApiError that is classified as retryable.
func retryableErrorf(format string, a ...any) ApiError {
	return &apiError{message: fmt.Sprintf(format, a...), retryAllowed: true}
}
