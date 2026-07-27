// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"net/url"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/handler/util/naming"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// reconcileInternalRoutes manages the optional managed routes for a zone.
func reconcileInternalRoutes(ctx context.Context, hc *HandlingContext) error {
	// Reset status fields related to team API routes to avoid stale data if routes are removed from spec
	hc.Zone.Status.TeamApiIdentityRealm = nil
	hc.Zone.Status.ManagedRoutes = nil
	hc.Zone.Status.Links.TeamIssuer = ""

	if hc.Zone.Spec.ManagedRoutes != nil {
		if err := reconcileTeamApiIdentityRealm(ctx, hc); err != nil {
			return err
		}

		if err := reconcileManagedRoutes(ctx, hc); err != nil {
			return err
		}
	}

	return nil
}

// reconcileTeamApiIdentityRealm creates the identity realm required by TeamAPI routes
// and derives the TeamIssuer link from the identity provider URL.
func reconcileTeamApiIdentityRealm(ctx context.Context, hc *HandlingContext) error {
	hasTeamRoutes := slices.ContainsFunc(hc.Zone.Spec.ManagedRoutes.Routes, func(route adminv1.ManagedRouteConfig) bool {
		return route.Type == adminv1.ManagedRouteTypeTeamAPI
	})
	if !hasTeamRoutes {
		return nil
	}

	opts := createIdentityRealmOptions{
		Claims:         hc.DefaultClaims,
		SecretRotation: nil, // TeamAPIs do not support rotation
	}
	teamApiIdentityRealm, err := createIdentityRealm(ctx, hc, naming.ForTeamApiIdentityRealm(hc.Environment), opts)
	if err != nil {
		return err
	}
	hc.TeamApiIdentityRealm = teamApiIdentityRealm
	hc.Zone.Status.TeamApiIdentityRealm = types.ObjectRefFromObject(teamApiIdentityRealm)

	// Derive TeamIssuer from identity provider URL + team identity realm name
	teamIssuer, err := url.JoinPath(hc.Zone.Spec.IdentityProvider.Url, "auth/realms/", teamApiIdentityRealm.Name)
	if err != nil {
		return ctrlerrors.BlockedErrorf("cannot build team issuer URL: %s", err)
	}
	hc.Zone.Status.Links.TeamIssuer = teamIssuer

	return nil
}

// reconcileManagedRoutes creates the gateway routes defined in the zone spec.
// All managed routes use the default gateway preset for hostname and path resolution.
func reconcileManagedRoutes(ctx context.Context, hc *HandlingContext) error {
	defaultPreset, err := hc.Zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return ctrlerrors.BlockedErrorf("managed routes require a default preset but none was found: %s", err)
	}

	// Partition routes by type
	var teamAPIRoutes, proxyRoutes []adminv1.ManagedRouteConfig
	for _, r := range hc.Zone.Spec.ManagedRoutes.Routes {
		switch r.Type {
		case adminv1.ManagedRouteTypeTeamAPI:
			teamAPIRoutes = append(teamAPIRoutes, r)
		case adminv1.ManagedRouteTypeProxy:
			proxyRoutes = append(proxyRoutes, r)
		default:
			return ctrlerrors.BlockedErrorf("unsupported managed route type %q for route %q", r.Type, r.Name)
		}
	}

	// TeamAPI routes: authenticated with disabled access control
	for _, routeConfig := range teamAPIRoutes {
		route, err := createManagedRoute(ctx, hc, routeConfig, defaultPreset, !EnablePassThrough)
		if err != nil {
			return err
		}
		hc.Zone.Status.ManagedRoutes = append(hc.Zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	// Proxy routes: full passthrough without authentication
	for _, routeConfig := range proxyRoutes {
		route, err := createManagedRoute(ctx, hc, routeConfig, defaultPreset, EnablePassThrough)
		if err != nil {
			return err
		}
		hc.Zone.Status.ManagedRoutes = append(hc.Zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	return nil
}

// createManagedRoute creates a single gateway route for a managed route configuration.
func createManagedRoute(ctx context.Context, hc *HandlingContext, routeConfig adminv1.ManagedRouteConfig, preset *adminv1.GatewayConfigPreset, passThrough bool) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hc.Gateway.Name + "--" + naming.ForGatewayRoute(routeConfig),
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

		upstreamUrl, err := url.Parse(routeConfig.Url)
		if err != nil {
			return ctrlerrors.BlockedErrorf("cannot parse upstream url of internal route %s: %s", routeConfig.Url, err)
		}
		upstream := gatewayapi.Upstream{
			Scheme:   upstreamUrl.Scheme,
			Hostname: upstreamUrl.Hostname(),
			Port:     gatewayapi.GetPortOrDefaultFromScheme(upstreamUrl),
			Path:     upstreamUrl.Path,
		}

		hostnames, paths := preset.ResolveHostnamesAndPaths(routeConfig.Path)

		route.Spec = gatewayapi.RouteSpec{
			Type:        gatewayapi.RouteTypePrimary,
			GatewayRef:  *types.ObjectRefFromObject(hc.Gateway),
			Backend:     gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:   hostnames,
			Paths:       paths,
			PassThrough: passThrough,
			Traffic:     gatewayapi.Traffic{},
		}

		if !passThrough {
			// Derive trusted issuer from the identity provider URL and team API identity realm
			trustedIssuer, issuerErr := url.JoinPath(hc.Zone.Spec.IdentityProvider.Url, "auth/realms/", hc.TeamApiIdentityRealm.Name)
			if issuerErr != nil {
				return ctrlerrors.BlockedErrorf("cannot build trusted issuer URL for route %s: %s", routeConfig.Name, issuerErr)
			}
			route.Spec.Security = gatewayapi.Security{
				DisableAccessControl: true,
				TrustedIssuers:       []string{trustedIssuer},
				RealmName:            hc.TeamApiIdentityRealm.Name,
			}
		}

		return nil
	}

	_, err := c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, ctrlerrors.RetryableErrorf("failed to create or update Gateway route %s in zone %s: %s", route.GetName(), hc.Zone.Name, err)
	}
	return route, nil
}
