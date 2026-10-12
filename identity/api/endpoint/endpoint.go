// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package endpoint derives Keycloak realm endpoints from an IdentityProvider admin URL.
// It is the single derivation used by the Identity operator for Realm snapshots and by
// Admin when comparing committed Realm snapshots with a new admin URL.
package endpoint

import "strings"

const (
	// MasterRealm is the Keycloak realm used for admin token requests.
	MasterRealm = "master"
	// TokenEndpointSuffix is appended to a realm issuer URL to form its token endpoint.
	TokenEndpointSuffix = "/protocol/openid-connect/token"
)

// IssuerURL returns the issuer URL of realmName. The admin URL is split at its first
// "/admin" segment; without one, "realms/<realmName>" is appended to the admin URL as is.
func IssuerURL(adminURL, realmName string) string {
	i := strings.Index(adminURL, "/admin")
	if i == -1 {
		return adminURL + "realms/" + realmName
	}
	return adminURL[:i] + "/realms/" + realmName
}

// TokenURL returns the token endpoint of realmName.
func TokenURL(adminURL, realmName string) string {
	return IssuerURL(adminURL, realmName) + TokenEndpointSuffix
}
