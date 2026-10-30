// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

func As[T client.CustomPlugin](s client.CustomPlugin, t *T) bool {
	if st, ok := s.(T); ok {
		*t = st
		return true
	}
	return false
}

// routeIdOf returns the Kong route ID recorded on the route, or "".
func routeIdOf(route *gatewayv1.Route) string {
	if route == nil {
		return ""
	}
	return route.GetRouteId()
}

// namePrefixOf returns the name prefix of the route, or "".
func namePrefixOf(route *gatewayv1.Route) string {
	if route == nil {
		return ""
	}
	return route.GetNamePrefix()
}
