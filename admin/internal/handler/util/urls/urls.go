// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package urls

import "strings"

func ensureSuffix(rawURL string) string {
	if !strings.HasSuffix(rawURL, "/") {
		return rawURL + "/"
	}
	return rawURL
}

func ForIdentityProviderAdminUrl(baseUrl string) string {
	adminUrl := ensureSuffix(baseUrl)

	return adminUrl + "auth/admin/realms"
}

func ForGatewayAdminUrl(baseUrl string) string {
	adminUrl := ensureSuffix(baseUrl)
	return adminUrl + "admin-api"
}

func ForGatewayAdminIssuerUrl(baseUrl string) string {
	adminIssuerUrl := ensureSuffix(baseUrl)

	return adminIssuerUrl + "auth/realms/rover"
}
