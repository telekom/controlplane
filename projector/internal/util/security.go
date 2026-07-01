// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

func MapCrBasicAuthToCpApi(basic *apiv1.BasicAuthCredentials) *model.BasicAuthCredentials {
	return &model.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

func MapCrOAuthToCpApi(oauth *apiv1.OAuth2ClientCredentials) *model.OAuth2ClientCredentials {
	return &model.OAuth2ClientCredentials{
		ClientId:     oauth.ClientId,
		ClientSecret: &oauth.ClientSecret,
		ClientKey:    &oauth.ClientKey,
	}
}

func MapCrExternalIdpToCpApi(externalIdp *apiv1.ExternalIdentityProvider) *model.ExternalIdentityProvider {
	tokenRequest := string(externalIdp.TokenRequest)
	return &model.ExternalIdentityProvider{
		TokenEndpoint: externalIdp.TokenEndpoint,
		TokenRequest:  &tokenRequest,
		GrantType:     &externalIdp.GrantType,
		Basic:         MapCrBasicAuthToCpApi(externalIdp.Basic),
		Client:        MapCrOAuthToCpApi(externalIdp.Client),
	}
}
