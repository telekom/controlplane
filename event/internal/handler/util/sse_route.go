// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

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

// How this works:
//
// Subscriber and Provider share the same Zone ("consumer" --> "foo" <-- "provider"):
// 1. Subscriber has deliveryType sse
// 2. Approval is done and Subscriber gets assigned the sseUrl "https://foo-gateway/horizon/sse/v1/<eventType>/<subscriptionId>"
// 3. Subscriber connect via the sseUrl to the foo-gateway which proxies the connection to Horizon and then forwards the events to the subscriber
//
// Subscriber is on a different zone ("consumer" --> "bar" --> "foo" <-- "provider"):
// 1. Subscriber has deliveryType sse
// 2. Approval is done and Subscriber gets assigned the sseUrl "https://bar-gateway/horizon/sse/v1/<eventType>/<subscriptionId>" (same path as the eventType is a singleton in the system)
// 3. Subscriber connect via the sseUrl to the bar-gateway which proxies the connection to foo-gateway which proxies the connection to Horizon and then forwards the events to the subscriber

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
	// isTargetOfProxy is authoritative for SSE; it drives the mesh-client consumer
	// via finalizeRoute's o.IsProxyTarget check.
	options.IsProxyTarget = isTargetOfProxy

	preset, err := resolvePreset(zone)
	if err != nil {
		return nil, err
	}

	// The primary SSE Route points at the zone's local Horizon backend. A proxy
	// zone has no local backend (Spec.Local == nil); callers must resolve the
	// target (local) zone and build the primary Route there instead.
	if eventConfig.Spec.Local == nil {
		return nil, ctrlerrors.BlockedErrorf("EventConfig %q for zone %q has no local backend; SSE primary Route requires a local (non-proxy) zone", eventConfig.Name, zone.Name)
	}

	upstream, err := parseUpstream(eventConfig.Spec.Local.ServerSendEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ServerSendEventUrl %q", eventConfig.Spec.Local.ServerSendEventUrl)
	}

	hostnames, paths := preset.ResolveHostnamesAndPaths(makeSSERoutePath(eventType))

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: zone.Status.Namespace,
		},
	}

	build := func() error {
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
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
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

	subscriberPreset, err := resolvePreset(subscriberZone)
	if err != nil {
		return nil, err
	}

	providerPreset, err := providerZone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no default preset: %s", providerZone.Name, err)
	}

	ssePath := makeSSERoutePath(eventType)
	upstream, err := gatewayUpstream(providerPreset, ssePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for SSE proxy route")
	}

	hostnames, paths := subscriberPreset.ResolveHostnamesAndPaths(ssePath)

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: subscriberZone.Status.Namespace,
		},
	}

	build := func() error {
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
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}
