// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/discovery-server/internal/api"
)

// TokenRequestCRDToAPI converts CRD tokenRequest values to discovery-server API enum values.
func TokenRequestCRDToAPI(value apiv1.TokenRequestMethod) api.OAuth2TokenRequest {
	switch value {
	case apiv1.TokenRequestClientSecretBasic:
		return api.Header
	case apiv1.TokenRequestClientSecretPost:
		return api.Body
	default:
		return api.OAuth2TokenRequest(value)
	}
}
