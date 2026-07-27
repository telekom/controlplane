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
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

// How this works:
//
// Provider is on a "real" zone ("provider" --> "foo"):
// 1. Provider has a publishEventUrl "https://foo-gateway/horizon/events/v1"
// 2. Provider published the events to this and thats it
//
// Provider is on a "proxy" zone ("provider" --> "bar" --> "foo"):
// 1. Provider has a publishEventUrl "https://bar-gateway/horizon/events/v1"
// 2. Provider published the events to this and bar-gateway forwards it to foo-gateway which then forwards it to the correct internal service

// CreatePublishRoute creates a Route for the publishing events
// The Route is created once per zone where the event-feature is configured
// and points to an internal service
func CreatePublishRoute(
	ctx context.Context,
	zone *adminv1.Zone,
	eventConfig *eventv1.EventConfig,
	opts ...Option,
) (*gatewayv1.Route, error) {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	preset, err := resolvePreset(zone)
	if err != nil {
		return nil, err
	}

	upstream, err := parseUpstream(eventConfig.Spec.Local.PublishEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse publishEventUrl %q", eventConfig.Spec.Local.PublishEventUrl)
	}

	// The publish route serves two downstream paths; the events path is first so it becomes the main path.
	hostnames, eventsPaths := preset.ResolveHostnamesAndPaths(makePublishEventsRoutePath())
	_, publishPaths := preset.ResolveHostnamesAndPaths(makePublishRoutePath())
	paths := slices.Concat(eventsPaths, publishPaths)

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makePublishRouteName(),
			Namespace: zone.Status.Namespace,
		},
	}

	build := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): zone.Name,
			config.BuildLabelKey("type"): "publish",
		}
		route.Spec = gatewayv1.RouteSpec{
			GatewayRef: *zone.Status.Gateway,
			Type:       gatewayv1.RouteTypePrimary,
			Backend:    gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayv1.Security{
				DisableAccessControl: true,
			},
		}
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}

// CreatePublishProxyRoute creates the publish Route for a proxy zone that runs no
// local event backend. Instead of pointing at an internal service, it is a proxy
// Route (mesh-client authenticated) whose upstream is the target zone's gateway.
// It mirrors the primary publish route's two downstream paths; the upstream carries
// the /horizon/events/v1 path, forwarding to the target's primary publish Route.
func CreatePublishProxyRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	opts ...Option,
) (*gatewayv1.Route, error) {
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

	// Upstream is the target zone's gateway publish-events path. The proxy forwards to
	// the target's primary publish Route at /horizon/events/v1.
	upstream, err := gatewayUpstream(targetPreset, makePublishEventsRoutePath())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy publish Route")
	}

	hostnames, eventsPaths := sourcePreset.ResolveHostnamesAndPaths(makePublishEventsRoutePath())
	_, publishPaths := sourcePreset.ResolveHostnamesAndPaths(makePublishRoutePath())
	paths := slices.Concat(eventsPaths, publishPaths)

	route := &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makePublishRouteName(),
			Namespace: sourceZone.Status.Namespace,
		},
	}

	build := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:        "event",
			config.BuildLabelKey("zone"): sourceZone.Name,
			config.BuildLabelKey("type"): "publish-proxy",
		}
		route.Spec = gatewayv1.RouteSpec{
			GatewayRef: *sourceZone.Status.Gateway,
			Type:       gatewayv1.RouteTypeProxy,
			Backend:    gatewayv1.Backend{Upstreams: []gatewayv1.Upstream{upstream}},
			Hostnames:  hostnames,
			Paths:      paths,
			Security: gatewayv1.Security{
				DisableAccessControl: true,
			},
		}
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}
