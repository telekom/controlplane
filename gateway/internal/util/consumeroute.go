// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

func HasM2MConsumeRoute(consumeRoute *gatewayv1.ConsumeRoute) bool {
	if consumeRoute.Spec.Security == nil {
		return false
	}
	return consumeRoute.Spec.Security.M2M != nil
}

func HasM2MExternalIdpConsumeRoute(consumeRoute *gatewayv1.ConsumeRoute) bool {
	if !HasM2MConsumeRoute(consumeRoute) {
		return false
	}
	return consumeRoute.Spec.Security.M2M.ExternalIDP != nil
}

func HasM2MExternalIdpClientConsumeRoute(consumeRoute *gatewayv1.ConsumeRoute) bool {
	if !HasM2MConsumeRoute(consumeRoute) {
		return false
	}
	if !HasM2MExternalIdpConsumeRoute(consumeRoute) {
		return false
	}
	return consumeRoute.Spec.Security.M2M.ExternalIDP.Client != nil
}
