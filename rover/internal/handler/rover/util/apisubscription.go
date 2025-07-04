// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	roverV1 "github.com/telekom/controlplane/rover/api/v1"
)

func SubscriptionHasM2M(apiSub *roverV1.ApiSubscription) bool {
	if apiSub.Security == nil {
		return false
	}

	return apiSub.Security.M2M != nil
}

func SubscriptionHasM2MClient(apiSub *roverV1.ApiSubscription) bool {
	if !SubscriptionHasM2M(apiSub) {
		return false
	}

	return apiSub.Security.M2M.Client != nil
}
