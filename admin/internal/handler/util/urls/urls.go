// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package urls

import (
	"net/url"
	"strings"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

func ensureSuffix(url string) string {
	if !strings.HasSuffix(url, "/") {
		return url + "/"
	}
	return url
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

func ForGatewayRealm(identityProviderBaseUrl string, realmName string) string {
	realmIssuerUrl := ensureSuffix(identityProviderBaseUrl)

	return realmIssuerUrl + "auth/realms/" + realmName
}

func ForRouteDownstream(gatewayBaseUrl string, config adminv1.ApiConfig) (*url.URL, error) {
	raw, err := url.JoinPath(gatewayBaseUrl, config.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot combine gatewayBaseUrl %s with team api route path %s", gatewayBaseUrl, config.Path)
	}
	return url.Parse(raw)
}

func ForStargateLmsIssuer(gatewayBaseUrl string, realmName string) string {
	return strings.TrimSuffix(gatewayBaseUrl, "/") + ":443" + "/auth/realms/" + realmName
}
