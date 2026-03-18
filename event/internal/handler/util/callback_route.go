// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func CreateProxyCallbackRoute(
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
	if err := condition.EnsureReady(downstreamRealm); err != nil {
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
	if err := condition.EnsureReady(upstreamRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", upstreamRealm.Name)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCallbackRouteName(targetZone.Name),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	downstream, err := downstreamRealm.AsDownstream(makeCallbackRoutePath(targetZone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for proxy callback Route")
	}

	upstream, err := upstreamRealm.AsUpstream(makeCallbackRoutePath(targetZone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy callback Route")
	}
	upstream.ClientId = meshClient.Spec.ClientId
	upstream.ClientSecret = meshClient.Spec.ClientSecret
	upstream.IssuerUrl = meshClient.Status.IssuerUrl

	mutator := func() error {

		err := options.apply(ctx, route)
		if err != nil {
			return errors.Wrap(err, "failed to apply options to proxy callback Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			config.BuildLabelKey("zone"):  sourceZone.Name,
			config.BuildLabelKey("realm"): downstreamRealm.Name,
			config.BuildLabelKey("type"):  "callback-proxy",
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
				// The mesh-client is used to access this Route
				DisableAccessControl: false,
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update proxy callback Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}

// CreateCallbackRoute creates a Route for sending callback events to subscribers
// The Route is created once per zone where the event-feature is configured
// and points to an internal service
func CreateCallbackRoute(
	ctx context.Context,
	zone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {

	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)
	name := makeCallbackRouteName(zone.Name)

	gatewayRealm := &gatewayapi.Realm{}
	err := c.Get(ctx, zone.Status.GatewayRealm.K8s(), gatewayRealm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", zone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", zone.Status.GatewayRealm.String())
	}
	if err := condition.EnsureReady(gatewayRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", gatewayRealm.Name)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: zone.Status.Namespace,
		},
	}

	upstream := gatewayapi.Upstream{
		Scheme: "http",
		Host:   "localhost",
		Path:   "/proxy",
		Port:   8080,
	}

	downstream, err := gatewayRealm.AsDownstream(makeCallbackRoutePath(zone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for callback Route")
	}
	mutator := func() error {

		err := options.apply(ctx, route)
		if err != nil {
			return errors.Wrap(err, "failed to apply options to callback Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			config.BuildLabelKey("zone"):  zone.Name,
			config.BuildLabelKey("realm"): gatewayRealm.Name,
			config.BuildLabelKey("type"):  "callback",
		}
		route.Spec = gatewayapi.RouteSpec{
			Realm: *ctypes.ObjectRefFromObject(gatewayRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Security: &gatewayapi.Security{
				// The mesh-client is used to access this Route
				DisableAccessControl: false,
			},
			Traffic: gatewayapi.Traffic{
				DynamicUpstream: &gatewayapi.DynamicUpstream{
					// Use DynamicUpstream to extract the actual callback URL from a query parameter at runtime
					QueryParameter: CallbackURLQueryParam,
				},
			},
		}
		if options.IsProxyTarget {
			// If this Route is used as target of a proxy Route,
			// the proxy-route will is the mesh-client. We need to allow access to this Route.
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, MeshClientName)
		}

		return nil
	}
	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update callback Route %q", ctypes.ObjectRefFromObject(route).String())
	}

	return route, nil
}

// CreateCallbackProxyRoutes creates cross-zone proxy Routes for callback delivery to remote subscribers.
// For each target zone, a Route is created in the source zone thats points to the target callback Route
// It is secured using OAuth2 credentials from the target zone's event service account.
func CreateCallbackProxyRoutes(
	ctx context.Context,
	meshConfig eventv1.MeshConfig,
	sourceZone *adminv1.Zone,
	targetZones []*adminv1.Zone,
	opts ...Option,
) (map[string]*gatewayapi.Route, error) {

	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	routes := map[string]*gatewayapi.Route{}
	zones := collectZones(targetZones, meshConfig.FullMesh, meshConfig.ZoneNames)
	logger.V(1).Info("Collected target zones for proxy callback Routes", "before", len(targetZones), "after", len(zones))

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

		route, err := CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create proxy callback Route for target zone %q", targetZone.Name)
		}
		routes[targetZone.Name] = route
		logger.V(1).Info("Created proxy callback Route for target zone", "targetZone", targetZone.Name, "route", ctypes.ObjectRefFromObject(route).String())
	}

	return routes, nil
}

// collectZones filters the given candidate zones based on the mesh configuration.
// If fullMesh is true, all candidates are returned.
func collectZones(candidates []*adminv1.Zone, fullMesh bool, wanted []string) []*adminv1.Zone {
	if fullMesh {
		return candidates
	}

	wantedSet := make(map[string]struct{})
	for _, name := range wanted {
		wantedSet[name] = struct{}{}
	}

	var collected []*adminv1.Zone
	for _, zone := range candidates {
		if _, ok := wantedSet[zone.Name]; ok {
			collected = append(collected, zone)
		}
	}

	return collected
}
