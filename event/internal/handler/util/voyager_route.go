// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

// CreateVoyagerRoute creates a gateway Route for the Voyager API endpoint.
// The Route is created once per zone where the event feature is configured
// and points to the internal Voyager backend service.
func CreateVoyagerRoute(
	ctx context.Context,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)
	name := makeVoyagerRouteName(zone.Name)

	gatewayRealm := &gatewayapi.Realm{}
	err := c.Get(ctx, zone.Status.GatewayRealm.K8s(), gatewayRealm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", zone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", zone.Status.GatewayRealm.String())
	}
	if err = condition.EnsureReady(gatewayRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", gatewayRealm.Name)
	}

	voyagerUrl, err := url.Parse(eventConfig.Spec.VoyagerApiUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse voyagerApiUrl %q", eventConfig.Spec.VoyagerApiUrl)
	}

	upstream := gatewayapi.Upstream{
		Scheme: voyagerUrl.Scheme,
		Host:   voyagerUrl.Hostname(),
		Port:   gatewayapi.GetPortOrDefaultFromScheme(voyagerUrl),
		Path:   voyagerUrl.Path,
	}

	meshDownstream, err := gatewayRealm.AsDownstream(makeVoyagerRoutePath(zone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mesh downstream for voyager Route")
	}

	localDownstream, err := gatewayRealm.AsDownstream(makeVoyagerRoutePath(""))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create local downstream for voyager Route")
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		if applyErr := options.apply(ctx, route); applyErr != nil {
			return errors.Wrap(applyErr, "failed to apply options to voyager Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			config.BuildLabelKey("zone"):  zone.Name,
			config.BuildLabelKey("realm"): gatewayRealm.Name,
			config.BuildLabelKey("type"):  "voyager",
		}
		route.Spec = gatewayapi.RouteSpec{
			Realm: *ctypes.ObjectRefFromObject(gatewayRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				meshDownstream,
				localDownstream,
			},
			Security: &gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		if options.IsProxyTarget {
			// If this Route is the target of a proxy Route,
			// the proxy-route uses the mesh-client. We need to allow access to this Route.
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, MeshClientName)
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update voyager Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}

// CreateProxyVoyagerRoute creates a single cross-zone proxy Route for the Voyager API.
// The Route is created in the source zone's namespace and points upstream
// to the target zone's gateway with OAuth2 credentials from the mesh client.
//
//nolint:dupl // parallel structure with CreateProxyCallbackRoute; differs in naming, labels, and security
func CreateProxyVoyagerRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	meshClient *identityv1.Client,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)

	downstreamRealm := &gatewayapi.Realm{}
	err := c.Get(ctx, sourceZone.Status.GatewayRealm.K8s(), downstreamRealm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", sourceZone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", sourceZone.Status.GatewayRealm.String())
	}
	if err = condition.EnsureReady(downstreamRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", downstreamRealm.Name)
	}

	upstreamRealm := &gatewayapi.Realm{}
	err = c.Get(ctx, targetZone.Status.GatewayRealm.K8s(), upstreamRealm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", targetZone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", targetZone.Status.GatewayRealm.String())
	}
	if err = condition.EnsureReady(upstreamRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", upstreamRealm.Name)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeVoyagerRouteName(targetZone.Name),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	downstream, err := downstreamRealm.AsDownstream(makeVoyagerRoutePath(targetZone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for proxy voyager Route")
	}

	upstream, err := upstreamRealm.AsUpstream(makeVoyagerRoutePath(targetZone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy voyager Route")
	}
	upstream.ClientId = meshClient.Spec.ClientId
	upstream.ClientSecret = meshClient.Spec.ClientSecret
	upstream.IssuerUrl = meshClient.Status.IssuerUrl

	mutator := func() error {
		if applyErr := options.apply(ctx, route); applyErr != nil {
			return errors.Wrap(applyErr, "failed to apply options to proxy voyager Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			config.BuildLabelKey("zone"):  sourceZone.Name,
			config.BuildLabelKey("realm"): downstreamRealm.Name,
			config.BuildLabelKey("type"):  "voyager-proxy",
		}
		route.Spec = gatewayapi.RouteSpec{
			Realm: *ctypes.ObjectRefFromObject(downstreamRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Security: &gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update proxy voyager Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}

// CreateVoyagerProxyRoutes creates cross-zone proxy Routes for the Voyager API.
// For each target zone, a Route is created in the source zone that points to
// the target zone's Voyager Route via the target zone's gateway with OAuth2 credentials.
//
//nolint:dupl // parallel structure with CreateCallbackProxyRoutes; differs in route type and security
func CreateVoyagerProxyRoutes(
	ctx context.Context,
	meshConfig *eventv1.MeshConfig,
	sourceZone *adminv1.Zone,
	targetZones []*adminv1.Zone,
	opts ...Option,
) (map[string]*gatewayapi.Route, error) {
	if meshConfig == nil {
		return nil, ctrlerrors.BlockedErrorf("meshConfig must not be nil")
	}

	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	routes := map[string]*gatewayapi.Route{}
	zones := collectZones(targetZones, meshConfig.FullMesh, meshConfig.ZoneNames)
	logger.V(1).Info("Collected target zones for proxy voyager Routes", "before", len(targetZones), "after", len(zones))

	for _, targetZone := range zones {
		if ctypes.Equals(sourceZone, targetZone) {
			// ignore the source zone itself if it's included in the target zones (in case of full mesh)
			continue
		}

		// Get the mesh-client credentials for the target zone.
		meshClient := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{
				Name:      MeshClientName,
				Namespace: targetZone.Status.Namespace,
			},
		}

		err := c.Get(ctx, client.ObjectKeyFromObject(meshClient), meshClient)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get mesh client credentials for target zone %q", targetZone.Name)
		}

		route, err := CreateProxyVoyagerRoute(ctx, sourceZone, targetZone, meshClient, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create proxy voyager Route for target zone %q", targetZone.Name)
		}
		routes[targetZone.Name] = route
		logger.V(1).Info("Created proxy voyager Route for target zone", "targetZone", targetZone.Name, "route", ctypes.ObjectRefFromObject(route).String())
	}

	return routes, nil
}
