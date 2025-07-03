// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import apiapi "github.com/telekom/controlplane/api/api/v1"

func HasExternalIdp(exposure *apiapi.ApiExposure) bool {

	if exposure.Spec.Security == nil {
		return false
	}
	if exposure.Spec.Security.M2M == nil {
		return false
	}
	if exposure.Spec.Security.M2M.ExternalIDP == nil {
		return false
	}

	return exposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint != ""
}
