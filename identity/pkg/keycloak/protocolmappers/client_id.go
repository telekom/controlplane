// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package protocolmappers

import (
	"github.com/telekom/controlplane/identity/pkg/api"
)

func toPtr[T any](v T) *T {
	return &v
}

func NewClientIdProtocolMapper() api.ProtocolMapperRepresentation {
	return api.ProtocolMapperRepresentation{
		Name:           toPtr("Client ID"),
		Protocol:       toPtr("openid-connect"),
		ProtocolMapper: toPtr("oidc-usersessionmodel-note-mapper"),
		Config: &map[string]interface{}{
			"user.session.note":    "clientId",
			"id.token.claim":       "true",
			"access.token.claim":   "true",
			"userinfo.token.claim": "false",
			"claim.name":           "clientId",
			"jsonType.label":       "String",
		},
	}
}
