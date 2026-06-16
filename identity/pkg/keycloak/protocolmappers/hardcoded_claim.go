// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package protocolmappers

import (
	"fmt"

	"github.com/telekom/controlplane/identity/pkg/api"
)

// NewHardcodedClaimMapper creates a protocol mapper that injects a static
// claim into ID tokens and access tokens. The mapper type used is
// Keycloak's built-in "oidc-hardcoded-claim-mapper".
func NewHardcodedClaimMapper(claimName, claimValue string) api.ProtocolMapperRepresentation {
	return api.ProtocolMapperRepresentation{
		Name:           toPtr(fmt.Sprintf("controlplane-claim-%s", claimName)),
		Protocol:       toPtr("openid-connect"),
		ProtocolMapper: toPtr("oidc-hardcoded-claim-mapper"),
		Config: &map[string]interface{}{
			claimNameConfigKey:          claimName,
			"claim.value":               claimValue,
			jsonTypeLabelConfigKey:      jsonTypeLabelString,
			idTokenClaimConfigKey:       stringTrue,
			accessTokenClaimConfigKey:   stringTrue,
			userInfoTokenClaimConfigKey: stringFalse,
		},
	}
}
