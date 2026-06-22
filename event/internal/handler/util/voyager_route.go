// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"
	"slices"

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

	// Resolve default preset for hostnames/paths
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default preset: %s", zone.Name, err)
	}
	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	upstream, err := parseUpstream(eventConfig.Spec.VoyagerApiUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse voyagerApiUrl %q", eventConfig.Spec.VoyagerApiUrl)
	}

	// The voyager route serves two paths: the mesh path (with zone name) and the local path (without zone name)
	meshHostnames, meshPaths := preset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(zone.Name))
	_, localPaths := preset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(""))

	// Hostnames are the same for both paths (from the same preset), so we only use one set.
	// Paths are different (mesh vs local) so we combine them.
	allHostnames := meshHostnames
	allPaths := slices.Concat(meshPaths, localPaths)

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
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "voyager",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  allHostnames,
			Paths:      allPaths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		options.applySecurity(route)
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
// to the target zone's gateway URL for the voyager path.
//
//nolint:dupl // parallel structure with CreateProxyCallbackRoute; differs in naming, labels, and security
func CreateProxyVoyagerRoute(
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
			Name:      makeVoyagerRouteName(targetZone.Name),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	// Build upstream: points at target zone's gateway URL for voyager path
	voyagerPath := makeVoyagerRoutePath(targetZone.Name)
	upstreamUrl, err := url.JoinPath(targetPreset.Urls[0].GetFullUrl(), voyagerPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build upstream URL for proxy voyager Route")
	}
	upstream, err := parseUpstream(upstreamUrl)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy voyager Route")
	}

	hostnames, paths := sourcePreset.ResolveHostnamesAndPaths(voyagerPath)

	mutator := func() error {
		if applyErr := options.apply(ctx, route); applyErr != nil {
			return errors.Wrap(applyErr, "failed to apply options to proxy voyager Route")
		}

		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): sourceZone.Name,
			config.BuildLabelKey("type"): "voyager-proxy",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *sourceZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		options.applySecurity(route)
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
// the target zone's Voyager Route via the target zone's gateway.
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

	routes := map[string]*gatewayapi.Route{}
	zones := collectZones(targetZones, meshConfig.FullMesh, meshConfig.ZoneNames)
	logger.V(1).Info("Collected target zones for proxy voyager Routes", "before", len(targetZones), "after", len(zones))

	for _, targetZone := range zones {
		if ctypes.Equals(sourceZone, targetZone) {
			// ignore the source zone itself if it's included in the target zones (in case of full mesh)
			continue
		}

		route, err := CreateProxyVoyagerRoute(ctx, sourceZone, targetZone, opts...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create proxy voyager Route for target zone %q", targetZone.Name)
		}
		routes[targetZone.Name] = route
		logger.V(1).Info("Created proxy voyager Route for target zone", "targetZone", targetZone.Name, "route", ctypes.ObjectRefFromObject(route).String())
	}

	return routes, nil
}
