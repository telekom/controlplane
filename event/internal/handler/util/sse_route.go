// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// CreateSSERoute creates a gateway Route for the SSE endpoint of an event type.
// The Route is created in the zone's namespace (cross-namespace from EventExposure),
// so NO owner reference is set. Uses c.CreateOrUpdate() which automatically tracks
// the Route in the JanitorClient state for later cleanup.
func CreateSSERoute(
	ctx context.Context,
	eventType string,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	isTargetOfProxy bool,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)

	// Resolve default preset for hostnames/paths
	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no default preset: %s", zone.Name, err)
	}
	if zone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no gateway reference in status", zone.Name)
	}

	// Build upstream from eventConfig.Spec.ServerSendEventUrl
	upstream, err := parseUpstream(eventConfig.Spec.ServerSendEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ServerSendEventUrl %q", eventConfig.Spec.ServerSendEventUrl)
	}

	hostnames, paths := preset.ResolveHostnamesAndPaths(makeSSERoutePath(eventType))

	// Create or update the Route
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			eventv1.EventTypeLabelKey:    labelutil.NormalizeLabelValue(eventType),
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "sse",
		}

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayapi.RouteTypePrimary,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayapi.Buffering{
				DisableResponseBuffering: true,
			},
		}
		options.applySecurity(route)
		// If this Route is used as target of a proxy Route,
		// the proxy-route will is the mesh-client. We need to allow access to this Route.
		if isTargetOfProxy {
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, MeshClientName)
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}

// CleanupOldSSERoutes uses the JanitorClient's Cleanup() to delete any stale SSE Routes
// for the given event type that were NOT created/updated in this reconciliation cycle.
// This handles cross-zone transfers: when an EventExposure moves from zone-1 to zone-2,
// the new Route is created in zone-2 (tracked in janitor state), and this function
// deletes the old Route in zone-1 (not in janitor state).
func CleanupOldSSERoutes(ctx context.Context, eventType string) (int, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	deleted, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, []client.ListOption{
		client.MatchingLabels{
			eventv1.EventTypeLabelKey: labelutil.NormalizeLabelValue(eventType),
		},
	})
	if err != nil {
		return deleted, errors.Wrapf(err, "failed to cleanup old SSE Routes for event type %q", eventType)
	}

	return deleted, nil
}

// CreateSSEProxyRoute creates a cross-zone proxy Route for SSE delivery.
// The Route is created in the subscriber zone's namespace and points upstream
// to the provider zone's gateway URL for the SSE path.
// This allows subscribers in a remote zone to consume SSE events without
// direct access to the provider zone's internal SSE endpoint.
func CreateSSEProxyRoute(
	ctx context.Context,
	eventType string,
	subscriberZone *adminv1.Zone,
	providerZone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	c := cclient.ClientFromContextOrDie(ctx)

	// Resolve subscriber zone's default preset for downstream hostnames/paths
	subscriberPreset, err := subscriberZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber zone %q has no default preset: %s", subscriberZone.Name, err)
	}
	if subscriberZone.Status.Gateway == nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber zone %q has no gateway reference in status", subscriberZone.Name)
	}

	// Resolve provider zone's default preset for upstream URL
	providerPreset, err := providerZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no default preset: %s", providerZone.Name, err)
	}

	// Build upstream: points at provider zone's gateway URL for SSE path
	ssePath := makeSSERoutePath(eventType)
	upstreamUrl, err := url.JoinPath(providerPreset.Urls[0].GetFullUrl(), ssePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build upstream URL for SSE proxy route")
	}
	upstream, err := parseUpstream(upstreamUrl)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for SSE proxy route")
	}

	hostnames, paths := subscriberPreset.ResolveHostnamesAndPaths(ssePath)

	// Create or update the proxy Route in the subscriber zone's namespace
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: subscriberZone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			eventv1.EventTypeLabelKey:    labelutil.NormalizeLabelValue(eventType),
			config.BuildLabelKey("zone"): subscriberZone.Name,
			config.BuildLabelKey("type"): "sse-proxy",
		}

		route.Spec = gatewayapi.RouteSpec{
			GatewayRef: *subscriberZone.Status.Gateway,
			Type:       gatewayapi.RouteTypeProxy,
			Backend:    gatewayapi.Backend{Upstreams: []gatewayapi.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayapi.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayapi.Buffering{
				DisableResponseBuffering: true,
			},
		}
		options.applySecurity(route)
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE proxy Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}
