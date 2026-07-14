// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

//nolint:dupl // parallel structure with CreateProxyVoyagerRoute; differs in naming, labels, and security
func CreateProxyCallbackRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)

	// Resolve source zone's default preset for downstream hostnames/paths
	sourcePreset, err := sourceZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("source zone %q has no default preset: %s", sourceZone.Name, err)
	}
	if sourceZone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("source zone %q has no gateway reference in status", sourceZone.Name)
	}

	// Resolve target zone's default preset for upstream URL
	targetPreset, err := targetZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("target zone %q has no default preset: %s", targetZone.Name, err)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCallbackRouteName(targetZone.Name),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	// Build upstream: points at target zone's gateway URL for callback path
	callbackPath := makeCallbackRoutePath(targetZone.Name)
	upstreamUrl, err := url.JoinPath(targetPreset.GetDefaultUrl(), callbackPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build upstream URL for proxy callback Route")
	}
	upstream, err := parseUpstream(upstreamUrl)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy callback Route")
	}

	hostnames, paths := sourcePreset.ResolveHostnamesAndPaths(callbackPath)

	mutator := func() error {
		if applyErr := options.apply(ctx, route); applyErr != nil {
			return errors.Wrap(applyErr, "failed to apply options to proxy callback Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): sourceZone.Name,
			config.BuildLabelKey("type"): "callback-proxy",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *sourceZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
				// The mesh-client is used to access this Route
				DisableAccessControl: false,
			},
		}
		options.applySecurity(route)
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

	// Resolve default preset for hostnames/paths
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default preset: %s", zone.Name, err)
	}
	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: zone.Status.Namespace,
		},
	}

	upstream := gatewayapi.Upstream{
		Scheme:   "http",
		Hostname: "localhost",
		Path:     "/proxy",
		Port:     8080,
	}

	hostnames, paths := preset.ResolveHostnamesAndPaths(makeCallbackRoutePath(zone.Name))

	mutator := func() error {
		if applyErr := options.apply(ctx, route); applyErr != nil {
			return errors.Wrap(applyErr, "failed to apply options to callback Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "callback",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
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
		options.applySecurity(route)
		if options.IsProxyTarget {
			// If this Route is used as target of a proxy Route,
			// the proxy-route uses the mesh-client. We need to allow access to this Route.
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
//
//nolint:dupl // parallel structure with CreateVoyagerProxyRoutes; differs in route type and security
func CreateCallbackProxyRoutes(
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

	routes := map[string]*gatewayapi.Route{}
	zones := collectZones(targetZones, meshConfig.FullMesh, meshConfig.ZoneNames)
	logger.V(1).Info("Collected target zones for proxy callback Routes", "before", len(targetZones), "after", len(zones))

	for _, targetZone := range zones {
		if ctypes.Equals(sourceZone, targetZone) {
			// ignore the source zone itself if it's included in the target zones (in case of full mesh)
			continue
		}

		route, err := CreateProxyCallbackRoute(ctx, sourceZone, targetZone, opts...)
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
