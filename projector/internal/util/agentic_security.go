// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// MapAgenticBasicAuthToCPAPI converts an agentic CR's BasicAuthCredentials to
// the shared cpapi model type. Mirrors MapCRBasicAuthToCPAPI for the agentic
// domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticBasicAuthToCPAPI(basic *agenticv1.BasicAuthCredentials) *model.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &model.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

// MapAgenticOAuthToCPAPI converts an agentic CR's OAuth2ClientCredentials to
// the shared cpapi model type. Mirrors MapCROAuthToCPAPI for the agentic
// domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticOAuthToCPAPI(oauth *agenticv1.OAuth2ClientCredentials) *model.OAuth2ClientCredentials {
	if oauth == nil {
		return nil
	}
	return &model.OAuth2ClientCredentials{
		ClientID:     oauth.ClientId,
		ClientSecret: &oauth.ClientSecret,
		ClientKey:    &oauth.ClientKey,
	}
}

// MapAgenticExternalIDPToCPAPI converts an agentic CR's ExternalIdentityProvider
// to the shared cpapi model type. Mirrors MapCRExternalIDPToCPAPI for the
// agentic domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticExternalIDPToCPAPI(externalIDP *agenticv1.ExternalIdentityProvider) *model.ExternalIdentityProvider {
	if externalIDP == nil {
		return nil
	}
	tokenRequest := string(externalIDP.TokenRequest)
	grantType := externalIDP.GrantType
	return &model.ExternalIdentityProvider{
		TokenEndpoint: externalIDP.TokenEndpoint,
		TokenRequest:  &tokenRequest,
		GrantType:     &grantType,
		Basic:         MapAgenticBasicAuthToCPAPI(externalIDP.Basic),
		Client:        MapAgenticOAuthToCPAPI(externalIDP.Client),
	}
}
