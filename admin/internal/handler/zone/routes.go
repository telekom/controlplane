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
	"github.com/telekom/controlplane/admin/internal/handler/util/urls"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctrlerrors "github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// reconcileInternalRoutes manages the optional managed routes for a zone.
func reconcileInternalRoutes(ctx context.Context, hc *HandlingContext) error {
	c := cclient.ClientFromContextOrDie(ctx)

	// Reset status fields related to team API routes to avoid stale data if routes are removed from spec
	hc.Zone.Status.TeamApiIdentityRealm = nil
	hc.Zone.Status.TeamApiGatewayRealm = nil
	hc.Zone.Status.ManagedRoutes = nil
	hc.Zone.Status.Links.TeamIssuer = ""

	if hc.Zone.Spec.ManagedRoutes != nil {
		if err := reconcileTeamApiRealms(ctx, hc); err != nil {
			return err
		}

		if err := reconcileManagedRoutes(ctx, hc); err != nil {
			return err
		}
	}

	// Cleanup managed routes that were not created or updated during this reconciliation.
	// Using OwnedByLabel because routes live in a different namespace than the Zone CR.
	if _, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, cclient.OwnedByLabel(hc.Zone)); err != nil {
		return ctrlerrors.RetryableErrorf("failed to cleanup stale managed routes for zone %s: %s", hc.Zone.Name, err)
	}

	return nil
}

// reconcileTeamApiRealms creates the identity and gateway realms required by TeamAPI routes.
func reconcileTeamApiRealms(ctx context.Context, hc *HandlingContext) error {
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

	teamApiGatewayRealm, err := createGatewayRealm(ctx, hc, naming.ForTeamApiGatewayRealm(hc.Environment))
	if err != nil {
		return err
	}
	hc.TeamApiGatewayRealm = teamApiGatewayRealm
	hc.Zone.Status.TeamApiGatewayRealm = types.ObjectRefFromObject(teamApiGatewayRealm)
	if len(teamApiGatewayRealm.Spec.IssuerUrls) > 0 {
		hc.Zone.Status.Links.TeamIssuer = teamApiGatewayRealm.Spec.IssuerUrls[0]
	}

	return nil
}

// reconcileManagedRoutes creates the gateway routes defined in the zone spec.
func reconcileManagedRoutes(ctx context.Context, hc *HandlingContext) error {
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

	// TeamAPI routes require a dedicated identity and gateway realm
	if len(teamAPIRoutes) > 0 && hc.TeamApiGatewayRealm == nil {
		return ctrlerrors.BlockedErrorf("team API routes require a gateway realm but none was created")
	}
	for _, routeConfig := range teamAPIRoutes {
		route, err := createManagedRoute(ctx, hc, routeConfig, hc.TeamApiGatewayRealm, !EnablePassThrough)
		if err != nil {
			return err
		}
		hc.Zone.Status.ManagedRoutes = append(hc.Zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	// Proxy routes use the default gateway realm with full passthrough
	if len(proxyRoutes) > 0 && hc.DefaultGatewayRealm == nil {
		return ctrlerrors.BlockedErrorf("proxy routes require a gateway realm but none was created")
	}
	for _, routeConfig := range proxyRoutes {
		route, err := createManagedRoute(ctx, hc, routeConfig, hc.DefaultGatewayRealm, EnablePassThrough)
		if err != nil {
			return err
		}
		hc.Zone.Status.ManagedRoutes = append(hc.Zone.Status.ManagedRoutes, *types.ObjectRefFromObject(route))
	}

	return nil
}

// createManagedRoute creates a single gateway route for a managed route configuration.
func createManagedRoute(ctx context.Context, hc *HandlingContext, routeConfig adminv1.ManagedRouteConfig, gatewayRealm *gatewayapi.Realm, passThrough bool) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayRealm.Name + "--" + naming.ForGatewayRoute(routeConfig),
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
			Scheme: upstreamUrl.Scheme,
			Host:   upstreamUrl.Hostname(),
			Port:   gatewayapi.GetPortOrDefaultFromScheme(upstreamUrl),
			Path:   upstreamUrl.Path,
		}

		downstreamUrl, err := urls.ForRouteDownstream(hc.Zone.Spec.Gateway.Url, routeConfig)
		if err != nil {
			return ctrlerrors.BlockedErrorf("cannot build downstream URL for route %s: %s", routeConfig.Name, err)
		}
		issuerUrl := ""
		if !passThrough && len(gatewayRealm.Spec.IssuerUrls) > 0 {
			issuerUrl = gatewayRealm.Spec.IssuerUrls[0]
		}
		downstream := gatewayapi.Downstream{
			Host:      downstreamUrl.Host,
			Port:      0,
			Path:      downstreamUrl.Path,
			IssuerUrl: issuerUrl,
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm:       *types.ObjectRefFromObject(gatewayRealm),
			PassThrough: passThrough,
			Upstreams:   []gatewayapi.Upstream{upstream},
			Downstreams: []gatewayapi.Downstream{downstream},
			Traffic:     gatewayapi.Traffic{},
		}

		if !passThrough {
			route.Spec.Security = &gatewayapi.Security{
				DisableAccessControl: true,
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
