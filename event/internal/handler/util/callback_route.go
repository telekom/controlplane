// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// How this works:
//
// Subscriber and Provider share the same Zone ("provider" --> "foo" --> "consumer"):
// 1. Subscriber has deliveryType callback with the callbackUrl "https://my-callback/v1/post"
// 2. Approval is done and pubsub-Subscription is provisioned with the callbackUrl "https://foo-gateway/horizon-foo/callback/v1?callback=https://my-callback/v1/post"
// 3. Horizon publishes events (auth using mesh-client) which will be proxies to the foo-gateway and then forwarded the the callbackUrl with the correct LMS-token (clientId=mesh-client)
//
// Subscriber is on a different zone ("provider" --> "bar" --> "foo" --> "consumer"):
// 1. Subscriber has deliveryType callback with the callbackUrl "https://my-callback/v1/post"
// 2. Approval is done and pubsub-Subscription is provisioned with the callbackUrl "https://bar-gateway/horizon-foo/callback/v1?callback=https://my-callback/v1/post"
// 3. Horizon publishes events (auth using mesh-client) to bar-gateway which is proxies to foo-gateway and then forwarded the the callbackUrl with the correct LMS-token (clientId=mesh-client)

func CreateProxyCallbackRoute(
	ctx context.Context,
	sourceZone *adminv1.Zone,
	targetZone *adminv1.Zone,
	opts ...Option,
) (*gatewayapi.Route, error) {
	return buildCrossZoneProxyRoute(ctx, sourceZone, targetZone, "callback",
		makeCallbackRouteName(targetZone.Name), makeCallbackRoutePath(targetZone.Name), false,
		append(opts, WithCallbackConsumer())...)
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
	// Callback routes always trust the Horizon callback client, regardless of
	// whether they are also a cross-zone proxy target.
	WithCallbackConsumer()(options)

	preset, err := resolvePreset(zone)
	if err != nil {
		return nil, err
	}

	upstream := gatewayapi.Upstream{
		Scheme:   "http",
		Hostname: "localhost",
		Path:     "/proxy",
		Port:     8080,
	}

	hostnames, paths := preset.ResolveHostnamesAndPaths(makeCallbackRoutePath(zone.Name))

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCallbackRouteName(zone.Name),
			Namespace: zone.Status.Namespace,
		},
	}

	build := func() error {
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
		return nil
	}
	return finalizeRoute(ctx, route, options, build)
}

// CreateCallbackProxyRoutes creates cross-zone proxy Routes for callback delivery to remote subscribers.
// For each target zone, a Route is created in the source zone thats points to the target callback Route
// It is secured using OAuth2 credentials from the target zone's event service account.
func CreateCallbackProxyRoutes(
	ctx context.Context,
	meshConfig *eventv1.MeshConfig,
	sourceZone *adminv1.Zone,
	targetZones []*adminv1.Zone,
	opts ...Option,
) (map[string]*gatewayapi.Route, error) {
	return createProxyRoutes(ctx, meshConfig, sourceZone, targetZones, CreateProxyCallbackRoute, opts...)
}
