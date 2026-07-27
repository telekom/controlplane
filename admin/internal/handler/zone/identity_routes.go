// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// identityRouteConfig describes one of the three OIDC-related passthrough routes
// (issuer, certs, discovery) that expose identity provider endpoints through the gateway.
type identityRouteConfig struct {
	// suffix is appended to the route name (e.g. "issuer", "certs", "discovery")
	suffix string
	// upstreamPathFmt is the path served by the Jumper identity container (port 8081).
	// The %s placeholder is replaced with the realm name.
	upstreamPathFmt string
	// downstreamPathFmt is the path exposed on the gateway.
	// The %s placeholder is replaced with the realm name.
	downstreamPathFmt string
}

// identityRouteConfigs lists the three identity routes created for each realm.
var identityRouteConfigs = []identityRouteConfig{
	{
		suffix:            "issuer",
		upstreamPathFmt:   "/api/v1/issuer/%s",
		downstreamPathFmt: "/auth/realms/%s",
	},
	{
		suffix:            "certs",
		upstreamPathFmt:   "/api/v1/certs/%s",
		downstreamPathFmt: "/auth/realms/%s/protocol/openid-connect/certs",
	},
	{
		suffix:            "discovery",
		upstreamPathFmt:   "/api/v1/discovery/%s",
		downstreamPathFmt: "/auth/realms/%s/.well-known/openid-configuration",
	},
}

const jumperIdentityPort int32 = 8081

// createIdentityRoutes creates the OIDC identity routes (issuer, certs, discovery) for
// the default identity realm and, if configured, the team-api identity realm.
// These passthrough routes allow external consumers to discover JWKS keys and validate
// last-mile-security tokens issued by this zone's gateway.
func createIdentityRoutes(ctx context.Context, hc *HandlingContext) error {
	defaultPreset, err := hc.Zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot resolve default gateway preset for identity routes: %s", err)
	}

	// World-visible zones prefix identity routes with /spacegate to avoid path conflicts
	var pathPrefix string
	if hc.Zone.Spec.Visibility == adminv1.ZoneVisibilityWorld {
		pathPrefix = spacegatePathPrefix
	}

	// Create routes for the default identity realm
	for _, cfg := range identityRouteConfigs {
		if err := createIdentityRoute(ctx, hc, hc.DefaultIdentityRealm.Name, cfg, defaultPreset, pathPrefix); err != nil {
			return err
		}
	}

	// Create routes for the team-api identity realm if it was set up
	if hc.TeamApiIdentityRealm != nil {
		for _, cfg := range identityRouteConfigs {
			if err := createIdentityRoute(ctx, hc, hc.TeamApiIdentityRealm.Name, cfg, defaultPreset, pathPrefix); err != nil {
				return err
			}
		}
	}

	return nil
}

// createIdentityRoute creates a single passthrough route that exposes an OIDC endpoint
// through the gateway, proxying to the Jumper identity container.
func createIdentityRoute(ctx context.Context, hc *HandlingContext, realmName string, cfg identityRouteConfig, preset *adminv1.GatewayConfigPreset, pathPrefix string) error {
	c := cclient.ClientFromContextOrDie(ctx)

	// Route name: gateway--<realmName>--<suffix>
	routeName := hc.Gateway.Name + "--" + realmName + "--" + cfg.suffix

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: hc.Namespace.Name,
		},
	}

	mutator := func() error {
		if route.Labels == nil {
			route.Labels = make(map[string]string)
		}
		route.Labels[cconfig.EnvironmentLabelKey] = hc.Environment.Name
		route.Labels[cconfig.BuildLabelKey(zoneLabelName)] = hc.Zone.Name
		route.Labels[cconfig.OwnerUidLabelKey] = string(hc.Zone.GetUID())

		// Construct the downstream path with optional spacegate prefix
		downstreamPath := fmt.Sprintf(cfg.downstreamPathFmt, realmName)
		if pathPrefix != "" {
			downstreamPath = pathPrefix + downstreamPath
		}
		hostnames, paths := preset.ResolveHostnamesAndPaths(downstreamPath)

		// Upstream: Jumper identity container on port 8081
		upstream := gatewayapi.Upstream{
			Scheme:   "http",
			Hostname: "localhost",
			Port:     jumperIdentityPort,
			Path:     fmt.Sprintf(cfg.upstreamPathFmt, realmName),
		}

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef:  *types.ObjectRefFromObject(hc.Gateway),
			Type:        gatewayapi.RouteTypePrimary,
			Backend:     gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:   hostnames,
			Paths:       paths,
			PassThrough: true,
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return ctrlerrors.RetryableErrorf("failed to create or update identity route %s in zone %s: %s", routeName, hc.Zone.Name, err)
	}
	return nil
}

// cleanupStaleRoutes removes any routes in the zone namespace that are owned by this zone
// but were not created or updated during the current reconciliation cycle.
// This covers both managed routes and identity routes.
func cleanupStaleRoutes(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	if _, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, cclient.OwnedByLabel(hc.Zone)); err != nil {
		return ctrlerrors.RetryableErrorf("failed to cleanup stale routes for zone %s: %s", hc.Zone.Name, err)
	}
	return nil
}
