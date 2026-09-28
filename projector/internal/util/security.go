// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

func MapCRBasicAuthToCPAPI(basic *apiv1.BasicAuthCredentials) *model.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &model.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

func MapCROAuthToCPAPI(oauth *apiv1.OAuth2ClientCredentials) *model.OAuth2ClientCredentials {
	if oauth == nil {
		return nil
	}
	return &model.OAuth2ClientCredentials{
		ClientID:     oauth.ClientId,
		ClientSecret: &oauth.ClientSecret,
		ClientKey:    &oauth.ClientKey,
	}
}

func MapCRExternalIDPToCPAPI(externalIDP *apiv1.ExternalIdentityProvider) *model.ExternalIdentityProvider {
	if externalIDP == nil {
		return nil
	}
	tokenRequest := string(externalIDP.TokenRequest)
	grantType := string(externalIDP.GrantType)
	return &model.ExternalIdentityProvider{
		TokenEndpoint: externalIDP.TokenEndpoint,
		TokenRequest:  &tokenRequest,
		GrantType:     &grantType,
		Basic:         MapCRBasicAuthToCPAPI(externalIDP.Basic),
		Client:        MapCROAuthToCPAPI(externalIDP.Client),
	}
}
