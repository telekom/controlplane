// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import apiapi "github.com/telekom/controlplane/api/api/v1"

func HasM2M(apiSub *apiapi.ApiSubscription) bool {
	if apiSub.Spec.Security == nil {
		return false
	}

	return apiSub.Spec.Security.M2M != nil
}

func HasM2MClient(apiSub *apiapi.ApiSubscription) bool {
	if !HasM2M(apiSub) {
		return false
	}

	return apiSub.Spec.Security.M2M.Client != nil
}
