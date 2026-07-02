// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

func HasFailoverSecurity(route *gatewayv1.Route) bool {
	return route.Spec.Traffic.Failover != nil
}

func HasM2M(route *gatewayv1.Route) bool {
	return route.Spec.Security.M2M != nil
}

func HasDynamicUpstream(route *gatewayv1.Route) bool {
	return route.Spec.Traffic.DynamicUpstream != nil
}

func HasM2MExternalIdp(route *gatewayv1.Route) bool {
	return HasM2M(route) && route.Spec.Security.M2M.ExternalIDP != nil
}

func HasRateLimit(route *gatewayv1.Route) bool {
	return route.Spec.Traffic.RateLimit != nil
}
