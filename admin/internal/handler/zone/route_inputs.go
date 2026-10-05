// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"path"
	"slices"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

const (
	maxRouteHostnames = 20
	maxRoutePaths     = 10
)

func gatewayRoutePaths(basePaths []string, downstreamPath string) []string {
	paths := make([]string, 0, len(basePaths))
	for _, basePath := range basePaths {
		joined := path.Join(basePath, downstreamPath)
		if !slices.Contains(paths, joined) {
			paths = append(paths, joined)
		}
	}
	slices.Sort(paths)
	return paths
}

// Validate before either creator so permanent limits cannot become retryable
// CRD validation failures in the identity-route step.
func validateGatewayRouteInputs(_ context.Context, hc *HandlingContext) error {
	var prefix string
	if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
		prefix = spacegatePathPrefix
	}
	for _, gateway := range hc.Zone.Spec.Gateways {
		hostnames, basePaths := gatewayHostnamesAndBasePaths(&hc.Zone.Spec, gateway.Name)
		if len(hostnames) == 0 || len(hostnames) > maxRouteHostnames {
			return ctrlerrors.BlockedErrorf("gateway %q has %d hostnames, but a route requires 1 to %d",
				gateway.Name, len(hostnames), maxRouteHostnames)
		}
		if err := validateGatewayRoutePaths(gateway.Name, basePaths, zoneHealthPath); err != nil {
			return err
		}
		for _, realmName := range identityRouteRealmNames(hc) {
			for _, cfg := range identityRouteConfigs {
				if err := validateGatewayRoutePaths(gateway.Name, basePaths, prefix+fmt.Sprintf(cfg.downstreamPathFmt, realmName)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateGatewayRoutePaths(gatewayName string, basePaths []string, downstreamPath string) error {
	paths := gatewayRoutePaths(basePaths, downstreamPath)
	if len(paths) == 0 || len(paths) > maxRoutePaths {
		return ctrlerrors.BlockedErrorf("gateway %q has %d paths for %q, but a route requires 1 to %d",
			gatewayName, len(paths), downstreamPath, maxRoutePaths)
	}
	return nil
}
