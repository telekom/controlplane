// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package protocolmappers

import (
	"fmt"

	"github.com/telekom/controlplane/identity/pkg/api"
)

// NewSessionNoteMapper creates a protocol mapper that reads a value from a
// Keycloak user-session note and injects it as a claim into ID tokens and
// access tokens. The mapper type used is Keycloak's built-in
// "oidc-usersessionmodel-note-mapper".
//
// Common session note keys populated automatically by Keycloak:
//   - "clientId"      — the OAuth2 client_id used in the token request
//   - "clientHost"    — hostname of the requesting client
//   - "clientAddress" — IP address of the requesting client
func NewSessionNoteMapper(claimName, sessionNoteKey string) api.ProtocolMapperRepresentation {
	return api.ProtocolMapperRepresentation{
		Name:           toPtr(fmt.Sprintf("controlplane-claim-%s", claimName)),
		Protocol:       toPtr("openid-connect"),
		ProtocolMapper: toPtr("oidc-usersessionmodel-note-mapper"),
		Config: &map[string]interface{}{
			"user.session.note":    sessionNoteKey,
			"claim.name":           claimName,
			"jsonType.label":       "String",
			"id.token.claim":       "true",
			"access.token.claim":   "true",
			"userinfo.token.claim": "false",
		},
	}
}
