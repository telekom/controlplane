// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"strings"

	"github.com/telekom/controlplane/identity/api/endpoint"
)

// AdminURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/"
// AdminConsoleURL example: "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/master/console/#/"
// IssuerURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/"
// AdminTokenURL example:
//			"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/protocol/openid-connect/token"

const (
	MasterRealm           = endpoint.MasterRealm
	TokenEndpointSuffix   = endpoint.TokenEndpointSuffix
	ConsoleEndpointSuffix = "/console/#/" // + realm name to directly open the realm in the console
)

func DetermineAdminConsoleUrlFrom(adminUrl, realmName string) string {
	i := strings.Index(adminUrl, "/realms")
	if i == -1 {
		return adminUrl + MasterRealm + ConsoleEndpointSuffix + realmName
	}
	return adminUrl[:i] + "/" + MasterRealm + ConsoleEndpointSuffix + realmName
}

// DetermineIssuerUrlFrom forwards to endpoint.IssuerURL.
func DetermineIssuerUrlFrom(adminUrl, realmName string) string {
	return endpoint.IssuerURL(adminUrl, realmName)
}

// DetermineAdminTokenUrlFrom forwards to endpoint.TokenURL.
func DetermineAdminTokenUrlFrom(adminUrl, realmName string) string {
	return endpoint.TokenURL(adminUrl, realmName)
}
