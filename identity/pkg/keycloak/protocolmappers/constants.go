// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package protocolmappers

//nolint:gosec // These are protocol mapper config keys, not credentials.
const (
	claimNameConfigKey          = "claim.name"
	jsonTypeLabelConfigKey      = "jsonType.label"
	jsonTypeLabelString         = "String"
	idTokenClaimConfigKey       = "id.token.claim"
	accessTokenClaimConfigKey   = "access.token.claim"
	userInfoTokenClaimConfigKey = "userinfo.token.claim"
	stringTrue                  = "true"
	stringFalse                 = "false"
)
