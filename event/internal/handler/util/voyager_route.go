// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// How this works:
//
// Subscriber and Provider share the same Zone ("consumer" --> "foo" <-- "provider"):
// 1. Subscriber wants to trigger the redelivery of events for a given eventType
// 2. Approval is done and Subscriber gets assigned the voyagerUrl "https://foo-gateway/horizon/voyager/v1/<...>"
// 3. Subscriber connect via the voyagerUrl to the foo-gateway which proxies the connection to Horizon-Voyager
//
// Subscriber is on a different zone ("consumer" --> "bar" --> "foo" <-- "provider"):
// 1. Subscriber wants to trigger the redelivery of events for a given eventType
// 2. Approval is done and Subscriber gets assigned the voyagerUrl "https://bar-gateway/horizon-foo/voyager/v1/<...>"
// 3. Subscriber connect via the voyagerUrl to the bar-gateway which proxies the connection to foo-gateway which proxies the connection to Horizon-Voyager
//
// Subscriber is on a proxy zone ("consumer" --> "bar" --> "foo" <-- "provider"):
// 1. Subscriber wants to trigger the redelivery of events for a given eventType
// 2. Approval is done and Subscriber gets assigned the voyagerUrl "https://bar-gateway/horizon/voyager/v1/<...>" (local voyagerUrl as the proxying is transparent to the subscriber)
// 3. Subscriber connect via the voyagerUrl to the bar-gateway which proxies the connection to foo-gateway which proxies the connection to Horizon-Voyager

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

	preset, err := resolvePreset(zone)
	if err != nil {
		return nil, err
	}

	upstream, err := parseUpstream(eventConfig.Spec.Local.VoyagerApiUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse voyagerApiUrl %q", eventConfig.Spec.Local.VoyagerApiUrl)
	}

	// The voyager route serves two paths: the local shortcut (without zone name) and the mesh
	// path (with zone name). The local path is first so it becomes the canonical VoyagerURL.
	meshHostnames, meshPaths := preset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(zone.Name))
	_, localPaths := preset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(""))
	allPaths := slices.Concat(localPaths, meshPaths)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeVoyagerRouteName(zone.Name),
			Namespace: zone.Status.Namespace,
		},
	}

	build := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "voyager",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  meshHostnames,
			Paths:      allPaths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}

// CreateProxyLocalVoyagerRoute creates the own-zone Voyager Route for a proxy zone.
// Like CreateVoyagerRoute it serves both the mesh path (/horizon-{sourceZone}/voyager/v1) and the
// local shortcut (/horizon/voyager/v1), but as a proxy Route whose upstream is the target zone's
// gateway Voyager path instead of a local backend. The proxy zone runs no Voyager backend
// of its own; reads are forwarded to the target zone. Downstream callers authenticate with
// the source zone's IDP tokens (same as a local primary Route); the gateway re-authenticates
// to the target with the mesh client.
func CreateProxyLocalVoyagerRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	sourcePreset, err := resolvePreset(sourceZone)
	if err != nil {
		return nil, err
	}

	targetPreset, err := targetZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("target zone %q has no default preset: %s", targetZone.Name, err)
	}

	// Upstream: target zone's gateway Voyager path (the target's primary Route).
	upstream, err := gatewayUpstream(targetPreset, makeVoyagerRoutePath(targetZone.Name))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy local voyager Route")
	}

	// Downstream serves both the local shortcut and the mesh path (with source zone name).
	// The local path is first so it becomes the canonical VoyagerURL.
	meshHostnames, meshPaths := sourcePreset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(sourceZone.Name))
	_, localPaths := sourcePreset.ResolveHostnamesAndPaths(makeVoyagerRoutePath(""))
	allPaths := slices.Concat(localPaths, meshPaths)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeVoyagerRouteName(sourceZone.Name),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	build := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): sourceZone.Name,
			config.BuildLabelKey("type"): "voyager",
		}
		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *sourceZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  meshHostnames,
			Paths:      allPaths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
		}
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}

// CreateProxyVoyagerRoute creates a single cross-zone proxy Route for the Voyager API.
// The Route is created in the source zone's namespace and points upstream
// to the target zone's gateway URL for the voyager path.
func CreateProxyVoyagerRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {
	return buildCrossZoneProxyRoute(ctx, sourceZone, targetZone, "voyager",
		makeVoyagerRouteName(targetZone.Name), makeVoyagerRoutePath(targetZone.Name), true, opts...)
}

// CreateVoyagerProxyRoutes creates cross-zone proxy Routes for the Voyager API.
// For each target zone, a Route is created in the source zone that points to
// the target zone's Voyager Route via the target zone's gateway.
func CreateVoyagerProxyRoutes(
	ctx context.Context,
	meshConfig *eventv1.MeshConfig,
	sourceZone *adminv1.Zone,
	targetZones []*adminv1.Zone,
	opts ...Option,
) (map[string]*gatewayapi.Route, error) {
	return createProxyRoutes(ctx, meshConfig, sourceZone, targetZones, CreateProxyVoyagerRoute, opts...)
}
