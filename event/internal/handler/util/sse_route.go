// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Nil-check zone.Status.GatewayRealm
	if zone.Status.GatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no GatewayRealm configured", zone.Name)
	}

	// 2. Get Realm CR
	realm := &gatewayapi.Realm{}
	if err := c.Get(ctx, zone.Status.GatewayRealm.K8s(), realm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("realm %q not found", zone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get realm %q", zone.Status.GatewayRealm.String())
	}

	// 3. Ensure realm is ready
	if err := condition.EnsureReady(realm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", realm.Name)
	}

	// 4. Build downstream
	downstream, err := realm.AsDownstream(makeSSERoutePath(eventType))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream")
	}

	// 5. Build upstream from eventConfig.Spec.ServerSendEventUrl
	parsedUrl, err := url.Parse(eventConfig.Spec.ServerSendEventUrl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ServerSendEventUrl %q", eventConfig.Spec.ServerSendEventUrl)
	}
	upstream := gatewayapi.Upstream{
		Scheme: parsedUrl.Scheme,
		Host:   parsedUrl.Hostname(),
		Port:   gatewayapi.GetPortOrDefaultFromScheme(parsedUrl),
		Path:   parsedUrl.Path,
	}

	// 6. Create or update the Route
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			eventv1.EventTypeLabelKey:     labelutil.NormalizeLabelValue(eventType),
			config.BuildLabelKey("zone"):  zone.Name,
			config.BuildLabelKey("realm"): realm.Name,
			config.BuildLabelKey("type"):  "sse",
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: *ctypes.ObjectRefFromObject(realm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Security: &gatewayapi.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayapi.Buffering{
				DisableResponseBuffering: true,
			},
		}
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
// to the provider zone's gateway with OAuth2 credentials.
// This allows subscribers in a remote zone to consume SSE events without
// direct access to the provider zone's internal SSE endpoint.
func CreateSSEProxyRoute(
	ctx context.Context,
	eventType string,
	evenConfig *eventv1.EventConfig,
	subscriberZone *adminv1.Zone,
	providerZone *adminv1.Zone,
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Resolve subscriber zone's realm (for downstream)
	if subscriberZone.Status.GatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber zone %q has no GatewayRealm configured", subscriberZone.Name)
	}
	subscriberRealm := &gatewayapi.Realm{}
	if err := c.Get(ctx, subscriberZone.Status.GatewayRealm.K8s(), subscriberRealm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("subscriber realm %q not found", subscriberZone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get subscriber realm %q", subscriberZone.Status.GatewayRealm.String())
	}
	if err := condition.EnsureReady(subscriberRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber realm %q is not ready", subscriberRealm.Name)
	}

	// 2. Resolve provider zone's realm (for upstream)
	if providerZone.Status.GatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no GatewayRealm configured", providerZone.Name)
	}
	providerRealm := &gatewayapi.Realm{}
	if err := c.Get(ctx, providerZone.Status.GatewayRealm.K8s(), providerRealm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("provider realm %q not found", providerZone.Status.GatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get provider realm %q", providerZone.Status.GatewayRealm.String())
	}
	if err := condition.EnsureReady(providerRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("provider realm %q is not ready", providerRealm.Name)
	}

	// 3. Build downstream from subscriber realm
	downstream, err := subscriberRealm.AsDownstream(makeSSERoutePath(eventType))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for proxy route")
	}

	// 4. Build upstream from provider realm with OAuth2 gateway credentials
	identityClient := &identityapi.Client{}
	if err := c.Get(ctx, evenConfig.Status.MeshClient.K8s(), identityClient); err != nil {
		return nil, errors.Wrapf(err, "failed to get gateway identity client for provider realm %s/%s",
			providerRealm.Name, providerRealm.Namespace)
	}

	upstream, err := providerRealm.AsUpstream(makeSSERoutePath(eventType))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy route")
	}
	upstream.ClientId = identityClient.Spec.ClientId
	upstream.ClientSecret = identityClient.Spec.ClientSecret
	upstream.IssuerUrl = identityClient.Status.IssuerUrl

	// 5. Create or update the proxy Route in the subscriber zone's namespace
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeSSERouteName(eventType),
			Namespace: subscriberZone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:         "event",
			eventv1.EventTypeLabelKey:     labelutil.NormalizeLabelValue(eventType),
			config.BuildLabelKey("zone"):  subscriberZone.Name,
			config.BuildLabelKey("realm"): subscriberRealm.Name,
			config.BuildLabelKey("type"):  "sse-proxy",
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: *ctypes.ObjectRefFromObject(subscriberRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
			Security: &gatewayapi.Security{
				DisableAccessControl: true,
			},
			Buffering: gatewayapi.Buffering{
				DisableResponseBuffering: true,
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update SSE proxy Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}
