// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import "strings"

// AdminURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/"
// AdminConsoleURL example: "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/master/console/#/"
// IssuerURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/"
// AdminTokenURL example:
//			"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/protocol/openid-connect/token"

const MasterRealm = "master"
const TokenEndpointSuffix = "/protocol/openid-connect/token"
const ConsoleEndpointSuffix = "/console/#/" // + realm name to directly open the realm in the console

func DetermineAdminConsoleUrlFrom(adminUrl string, realmName string) string {
	i := strings.Index(adminUrl, "/realms")
	if i == -1 {
		return adminUrl + MasterRealm + ConsoleEndpointSuffix + realmName
	}
	return adminUrl[:i] + "/" + MasterRealm + ConsoleEndpointSuffix + realmName
}

func DetermineIssuerUrlFrom(adminUrl, realmName string) string {
	i := strings.Index(adminUrl, "/admin")
	if i == -1 {
		return adminUrl + "realms/" + realmName
	}
	return adminUrl[:i] + "/realms/" + realmName
}

func DetermineAdminTokenUrlFrom(adminUrl, realmName string) string {
	return DetermineIssuerUrlFrom(adminUrl, realmName) + TokenEndpointSuffix
}
