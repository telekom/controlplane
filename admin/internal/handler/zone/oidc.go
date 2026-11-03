// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"fmt"
	"net/url"
	"strings"

	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

// keycloakTokenPathSuffix is the Keycloak token endpoint path relative to a realm issuer.
const keycloakTokenPathSuffix = "/protocol/openid-connect/token"

// defaultRealmTokenURL derives the token endpoint from the issuer the identity
// operator resolved for the default realm.
func defaultRealmTokenURL(hc *HandlingContext, presetName string) (string, error) {
	if hc.DefaultIdentityRealm == nil {
		return "", ctrlerrors.RetryableErrorf("cannot derive token URL for preset %q: default identity realm is not resolved", presetName)
	}
	issuer := strings.TrimRight(hc.DefaultIdentityRealm.Status.IssuerUrl, "/")
	if issuer == "" {
		return "", ctrlerrors.RetryableErrorf("cannot derive token URL for preset %q: default identity realm %q has no resolved issuer URL", presetName, hc.DefaultIdentityRealm.Name)
	}
	tokenURL := issuer + keycloakTokenPathSuffix
	if err := validateTokenURL(tokenURL); err != nil {
		return "", ctrlerrors.RetryableErrorf("cannot derive token URL for preset %q from default identity realm %q: %s", presetName, hc.DefaultIdentityRealm.Name, err)
	}
	return tokenURL, nil
}

func validateTokenURL(rawURL string) error {
	tokenURL, err := url.ParseRequestURI(rawURL)
	if err != nil || strings.Contains(rawURL, "#") || !tokenURL.IsAbs() || tokenURL.Scheme != "https" || tokenURL.Hostname() == "" || tokenURL.User != nil || tokenURL.Fragment != "" {
		return fmt.Errorf("token endpoint must be an absolute HTTPS URL without userinfo or fragment")
	}
	if tokenURL.RawQuery != "" || tokenURL.ForceQuery {
		return fmt.Errorf("token endpoint must not contain a query")
	}
	return nil
}
