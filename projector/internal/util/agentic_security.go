// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

// MapAgenticBasicAuthToCpApi converts an agentic CR's BasicAuthCredentials to
// the shared cpapi model type. Mirrors MapCrBasicAuthToCpApi for the agentic
// domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticBasicAuthToCpApi(basic *agenticv1.BasicAuthCredentials) *model.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &model.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

// MapAgenticOAuthToCpApi converts an agentic CR's OAuth2ClientCredentials to
// the shared cpapi model type. Mirrors MapCrOAuthToCpApi for the agentic
// domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticOAuthToCpApi(oauth *agenticv1.OAuth2ClientCredentials) *model.OAuth2ClientCredentials {
	if oauth == nil {
		return nil
	}
	return &model.OAuth2ClientCredentials{
		ClientId:     oauth.ClientId,
		ClientSecret: &oauth.ClientSecret,
		ClientKey:    &oauth.ClientKey,
	}
}

// MapAgenticExternalIdpToCpApi converts an agentic CR's ExternalIdentityProvider
// to the shared cpapi model type. Mirrors MapCrExternalIdpToCpApi for the
// agentic domain's (structurally identical, but nominally distinct) CRD type.
func MapAgenticExternalIdpToCpApi(externalIdp *agenticv1.ExternalIdentityProvider) *model.ExternalIdentityProvider {
	if externalIdp == nil {
		return nil
	}
	tokenRequest := string(externalIdp.TokenRequest)
	grantType := externalIdp.GrantType
	return &model.ExternalIdentityProvider{
		TokenEndpoint: externalIdp.TokenEndpoint,
		TokenRequest:  &tokenRequest,
		GrantType:     &grantType,
		Basic:         MapAgenticBasicAuthToCpApi(externalIdp.Basic),
		Client:        MapAgenticOAuthToCpApi(externalIdp.Client),
	}
}
