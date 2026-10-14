// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"fmt"
	"net/url"
	"strings"
)

// AdminURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/"
// AdminConsoleURL example: "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/master/console/#/"
// IssuerURL example: 		"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/"
// AdminTokenURL example:
//			"https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/protocol/openid-connect/token"

const (
	MasterRealm           = "master"
	TokenEndpointSuffix   = "/protocol/openid-connect/token"
	ConsoleEndpointSuffix = "/console/#/" // + realm name to directly open the realm in the console
)

func DetermineAdminConsoleUrlFrom(adminUrl, realmName string) string {
	i := strings.Index(adminUrl, "/realms")
	if i == -1 {
		return adminUrl + MasterRealm + ConsoleEndpointSuffix + realmName
	}
	return adminUrl[:i] + "/" + MasterRealm + ConsoleEndpointSuffix + realmName
}

// DetermineIssuerUrlFrom returns "<base>/realms/<realmName>", where base is
// the admin URL up to its first "admin" path segment (or the whole path if
// there is none). Scheme, host and context path are preserved; query and
// fragment are dropped. realmName is appended as is, so callers must escape it.
// The returned error does not contain the admin URL, which may hold credentials.
func DetermineIssuerUrlFrom(adminUrl, realmName string) (string, error) {
	u, err := parseUrl(adminUrl)
	if err != nil {
		return "", fmt.Errorf("invalid admin URL: %w", err)
	}
	segments := strings.Split(u.EscapedPath(), "/")
	for i, segment := range segments {
		if segment == "admin" {
			segments = segments[:i]
			break
		}
	}
	basePath := strings.TrimRight(strings.Join(segments, "/"), "/")
	base := url.URL{Scheme: u.Scheme, User: u.User, Host: u.Host}
	return base.String() + basePath + "/realms/" + realmName, nil
}

func DetermineAdminTokenUrlFrom(adminUrl, realmName string) (string, error) {
	issuerUrl, err := DetermineIssuerUrlFrom(adminUrl, realmName)
	if err != nil {
		return "", err
	}
	return issuerUrl + TokenEndpointSuffix, nil
}
