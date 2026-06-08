// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
)

// CreateMcpRoute creates the primary gateway Route for an MCP exposure.
// The Route is created in the zone's namespace with buffering disabled for streaming.
func CreateMcpRoute(
	ctx context.Context,
	exposure *agenticv1.McpExposure,
	zone *adminv1.Zone,
	isTargetOfProxy bool,
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Check zone has AiGatewayRealm
	if zone.Status.AiGatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no AiGatewayRealm configured — AI Gateway feature not enabled", zone.Name)
	}

	// 2. Get Realm CR
	realm := &gatewayapi.Realm{}
	if err := c.Get(ctx, zone.Status.AiGatewayRealm.K8s(), realm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("AI Gateway realm %q not found", zone.Status.AiGatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get AI Gateway realm %q", zone.Status.AiGatewayRealm.String())
	}

	// 3. Ensure realm is ready
	if err := condition.EnsureReady(realm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("AI Gateway realm %q is not ready", realm.Name)
	}

	// 4. Build downstream from realm
	downstream, err := realm.AsDownstream(exposure.Spec.McpBasePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream")
	}

	// 5. Create or update the Route
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeMcpRouteName(exposure.Spec.McpBasePath),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:         "agentic",
			agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(exposure.Spec.McpBasePath),
			config.BuildLabelKey("zone"):  zone.Name,
			config.BuildLabelKey("realm"): realm.Name,
			config.BuildLabelKey("type"):  "mcp",
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm:       *ctypes.ObjectRefFromObject(realm),
			Upstreams:   exposure.Spec.Upstreams,
			Downstreams: []gatewayapi.Downstream{downstream},
			// Critical: disable buffering for MCP streaming (SSE)
			Buffering: gatewayapi.Buffering{
				DisableRequestBuffering:  true,
				DisableResponseBuffering: true,
			},
		}

		// Apply security settings if provided, otherwise disable access control
		if exposure.Spec.Security != nil {
			route.Spec.Security = exposure.Spec.Security
		} else {
			route.Spec.Security = &gatewayapi.Security{
				DisableAccessControl: true,
			}
		}

		// Apply traffic settings if provided
		if exposure.Spec.Traffic != nil {
			route.Spec.Traffic = *exposure.Spec.Traffic
		}

		// If this Route is a target of proxy Routes, allow mesh-client access
		if isTargetOfProxy {
			if route.Spec.Security == nil {
				route.Spec.Security = &gatewayapi.Security{}
			}
			route.Spec.Security.DefaultConsumers = append(route.Spec.Security.DefaultConsumers, MeshClientName)
		}

		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update MCP Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}

// CreateMcpProxyRoute creates a cross-zone proxy Route for MCP delivery.
// The Route is created in the subscriber zone's namespace and points upstream
// to the provider zone's AI Gateway.
func CreateMcpProxyRoute(
	ctx context.Context,
	basePath string,
	subscriberZone *adminv1.Zone,
	providerZone *adminv1.Zone,
) (*gatewayapi.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Resolve subscriber zone's AI Gateway realm (for downstream)
	if subscriberZone.Status.AiGatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber zone %q has no AiGatewayRealm configured", subscriberZone.Name)
	}
	subscriberRealm := &gatewayapi.Realm{}
	if err := c.Get(ctx, subscriberZone.Status.AiGatewayRealm.K8s(), subscriberRealm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("subscriber AI Gateway realm %q not found", subscriberZone.Status.AiGatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get subscriber AI Gateway realm %q", subscriberZone.Status.AiGatewayRealm.String())
	}
	if err := condition.EnsureReady(subscriberRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("subscriber AI Gateway realm %q is not ready", subscriberRealm.Name)
	}

	// 2. Resolve provider zone's AI Gateway realm (for upstream)
	if providerZone.Status.AiGatewayRealm == nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no AiGatewayRealm configured", providerZone.Name)
	}
	providerRealm := &gatewayapi.Realm{}
	if err := c.Get(ctx, providerZone.Status.AiGatewayRealm.K8s(), providerRealm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("provider AI Gateway realm %q not found", providerZone.Status.AiGatewayRealm.String())
		}
		return nil, errors.Wrapf(err, "failed to get provider AI Gateway realm %q", providerZone.Status.AiGatewayRealm.String())
	}
	if err := condition.EnsureReady(providerRealm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("provider AI Gateway realm %q is not ready", providerRealm.Name)
	}

	// 3. Build downstream from subscriber realm
	downstream, err := subscriberRealm.AsDownstream(basePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create downstream for proxy route")
	}

	// 4. Build upstream from provider realm with gateway credentials
	// Get the gateway identity client for cross-zone mesh communication
	if providerZone.Status.GatewayClient == nil {
		return nil, ctrlerrors.BlockedErrorf("provider zone %q has no GatewayClient configured", providerZone.Name)
	}
	identityClient := &identityapi.Client{}
	if err = c.Get(ctx, providerZone.Status.GatewayClient.K8s(), identityClient); err != nil {
		return nil, errors.Wrapf(err, "failed to get gateway identity client for provider zone %q", providerZone.Name)
	}

	upstream, err := providerRealm.AsUpstream(basePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create upstream for proxy route")
	}
	upstream.ClientId = identityClient.Spec.ClientId
	upstream.ClientSecret = identityClient.Spec.ClientSecret
	upstream.IssuerUrl = identityClient.Status.IssuerUrl

	// 5. Create or update the proxy Route in the subscriber zone's namespace
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MakeMcpRouteName(basePath),
			Namespace: subscriberZone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			config.DomainLabelKey:         "agentic",
			agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
			config.BuildLabelKey("zone"):  subscriberZone.Name,
			config.BuildLabelKey("realm"): subscriberRealm.Name,
			config.BuildLabelKey("type"):  "mcp-proxy",
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
			// Critical: disable buffering for MCP streaming
			Buffering: gatewayapi.Buffering{
				DisableRequestBuffering:  true,
				DisableResponseBuffering: true,
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update MCP proxy Route %s/%s", route.Namespace, route.Name)
	}

	return route, nil
}

// CleanupOldMcpRoutes uses the JanitorClient's Cleanup() to delete stale MCP Routes
// for the given basePath that were NOT created/updated in this reconciliation cycle.
func CleanupOldMcpRoutes(ctx context.Context, basePath string) (int, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	deleted, err := c.Cleanup(ctx, &gatewayapi.RouteList{}, []client.ListOption{
		client.MatchingLabels{
			agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
		},
	})
	if err != nil {
		return deleted, errors.Wrapf(err, "failed to cleanup old MCP Routes for basePath %q", basePath)
	}

	return deleted, nil
}

// DeleteRouteIfExists deletes a Route by ObjectRef, ignoring NotFound errors.
func DeleteRouteIfExists(ctx context.Context, ref *ctypes.ObjectRef) error {
	c := cclient.ClientFromContextOrDie(ctx)
	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
	}
	err := c.Delete(ctx, route)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete Route %q", ref.String())
	}
	return nil
}
