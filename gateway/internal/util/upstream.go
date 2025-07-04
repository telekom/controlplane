// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

func HasM2MUpstream(upstream *gatewayv1.Upstream) bool {
	if upstream.Security == nil {
		return false
	}
	return upstream.Security.M2M != nil
}

func HasM2MExternalIdpUpstream(upstream *gatewayv1.Upstream) bool {
	if !HasM2MUpstream(upstream) {
		return false
	}
	return upstream.Security.M2M.ExternalIDP != nil
}

func HasM2MExternalIdpClientUpstream(upstream *gatewayv1.Upstream) bool {
	if !HasM2MUpstream(upstream) {
		return false
	}
	if !HasM2MExternalIdpUpstream(upstream) {
		return false
	}
	return upstream.Security.M2M.ExternalIDP.Client != nil
}
