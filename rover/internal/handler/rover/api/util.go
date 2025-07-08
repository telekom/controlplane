// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

func MakeName(ownerName, basePath, organization string) string {
	name := ownerName + "--" + strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
	if organization != "" {
		name = organization + "--" + name
	}

	return name
}

func toApiClient(client *rover.OAuth2ClientCredentials) *apiapi.OAuth2ClientCredentials {
	if client == nil {
		return nil
	}
	return &apiapi.OAuth2ClientCredentials{
		ClientId:     client.ClientId,
		ClientSecret: client.ClientSecret,
		Scopes:       client.Scopes,
	}
}

func toApiBasic(basic *rover.BasicAuthCredentials) *apiapi.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &apiapi.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}
