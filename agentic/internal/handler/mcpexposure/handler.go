// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpexposure

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

var _ handler.Handler[*agenticv1.McpExposure] = &McpExposureHandler{}

type McpExposureHandler struct {
	Config *agenticconfig.AgenticConfig
}

func (h *McpExposureHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.McpExposure) error {
	logger := log.FromContext(ctx)

	// 1. Validate McpServer exists and is active
	found, _, err := util.FindActiveMcpServer(ctx, obj.Spec.BasePath)
	if err != nil {
		return err
	}
	if !found {
		obj.SetCondition(condition.NewNotReadyCondition("McpServerNotFound",
			"No active McpServer found for basePath "+obj.Spec.BasePath))
		obj.SetCondition(condition.NewBlockedCondition(
			"McpServer " + obj.Spec.BasePath + " does not exist or is not active. " +
				"McpExposure will be automatically processed when the McpServer is registered"))
		return nil
	}

	// 2. Check for competing exposures (oldest-wins)
	existingExposures, err := util.FindMcpExposures(ctx, obj.Spec.BasePath)
	if err != nil {
		return errors.Wrapf(err, "failed to list McpExposures for basePath %q", obj.Spec.BasePath)
	}
	existingFound, existingExposure, err := util.FindActiveMcpExposure(existingExposures)
	if err != nil {
		return errors.Wrapf(err, "failed to find active McpExposure for basePath %q", obj.Spec.BasePath)
	}

	if existingFound && existingExposure.UID != obj.UID {
		obj.Status.Active = false
		msg := fmt.Sprintf("BasePath %q is already exposed by team %q.", obj.Spec.BasePath, existingExposure.Spec.Provider.Namespace)
		obj.SetCondition(condition.NewNotReadyCondition("McpExposureAlreadyExists", msg))
		obj.SetCondition(condition.NewBlockedCondition(msg + " McpExposure will be automatically processed when the existing one is deleted"))
		return nil
	}

	// This exposure is active
	obj.Status.Active = true

	// 3. Get and validate zone
	zone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return err
	}

	// 4. Check zone supports AI Gateway
	if !zone.IsFeatureEnabled(adminv1.FeatureAiGateway) {
		obj.SetCondition(condition.NewNotReadyCondition("AiGatewayNotSupported",
			"Zone "+zone.Name+" does not support the AI Gateway feature"))
		return ctrlerrors.BlockedErrorf("zone %q does not support the AI Gateway feature", zone.Name)
	}

	// 5. Handle cross-zone proxy routes
	obj.Status.Route = nil
	obj.Status.ProxyRoutes = nil

	crossZones, err := util.FindCrossZoneMcpSubscriptionZones(ctx, obj.Spec.BasePath, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrap(err, "failed to find cross-zone MCP subscriptions")
	}

	for _, subscriberZoneRef := range crossZones {
		subscriberZone, zoneErr := util.GetZone(ctx, subscriberZoneRef.K8s())
		if zoneErr != nil {
			return errors.Wrapf(zoneErr, "failed to get subscriber zone %q", subscriberZoneRef.Name)
		}

		proxyRoute, routeErr := util.CreateMcpProxyRoute(ctx, obj.Spec.BasePath, subscriberZone, zone)
		if routeErr != nil {
			return errors.Wrapf(routeErr, "failed to create MCP proxy Route for zone %q", subscriberZoneRef.Name)
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *ctypes.ObjectRefFromObject(proxyRoute))
		logger.V(1).Info("MCP proxy Route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	// 6. Resolve platform consumer for auto-access (TeleMCP variant)
	telecontextConsumer := ""
	if obj.Spec.Variant.IsTelecontextVariant() {
		if h.Config.TelecontextConsumerName == "" {
			return errors.New("TELECONTEXTMCP variant requires telecontext consumer name to be configured")
		}
		telecontextConsumer = h.Config.TelecontextConsumerName
	}

	// 7. Create primary MCP route
	isProxyTarget := len(obj.Status.ProxyRoutes) > 0
	route, err := util.CreateMcpRoute(ctx, obj, zone, isProxyTarget, telecontextConsumer)
	if err != nil {
		return errors.Wrap(err, "failed to create MCP Route")
	}
	obj.Status.Route = ctypes.ObjectRefFromObject(route)
	logger.V(1).Info("MCP Route created/updated", "route", route.Name)

	// 8. Cleanup stale routes
	deleted, err := util.CleanupOldMcpRoutes(ctx, obj.Spec.BasePath)
	if err != nil {
		return errors.Wrap(err, "failed to cleanup old MCP Routes")
	}
	if deleted > 0 {
		logger.V(1).Info("Cleaned up stale MCP Routes", "deleted", deleted)
	}

	// 9. Set final conditions
	c := cclient.ClientFromContextOrDie(ctx)
	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady",
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("McpExposureProvisioned",
		"McpExposure has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"McpExposure has been provisioned"))

	return nil
}

func (h *McpExposureHandler) Delete(ctx context.Context, obj *agenticv1.McpExposure) error {
	logger := log.FromContext(ctx)

	// Check if another McpExposure exists for the same basePath.
	otherExists, err := util.AnyOtherMcpExposureExists(ctx, obj.Spec.BasePath, obj.UID)
	if err != nil {
		return errors.Wrap(err, "failed to check for other McpExposures")
	}

	if otherExists {
		logger.Info("Skipping Route deletion — another McpExposure exists for this basePath",
			"basePath", obj.Spec.BasePath)
		return nil
	}

	// Last exposure for this basePath — clean up Routes and ConsumeRoutes.
	if obj.Status.Route != nil {
		if err := util.DeleteRouteIfExists(ctx, obj.Status.Route); err != nil {
			return errors.Wrap(err, "failed to delete MCP Route")
		}
		logger.Info("Deleted MCP Route", "route", obj.Status.Route.String())
	}

	for i := range obj.Status.ProxyRoutes {
		ref := &obj.Status.ProxyRoutes[i]
		if err := util.DeleteRouteIfExists(ctx, ref); err != nil {
			return errors.Wrapf(err, "failed to delete MCP proxy Route %q", ref.String())
		}
		logger.Info("Deleted MCP proxy Route", "route", ref.String())
	}

	return nil
}
