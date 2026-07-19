// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	"context"
	"fmt"
	"strings"

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

var _ handler.Handler[*agenticv1.AgenticExposure] = &AgenticExposureHandler{}

type AgenticExposureHandler struct {
	Config *agenticconfig.AgenticConfig
}

func (h *AgenticExposureHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.AgenticExposure) error {
	logger := log.FromContext(ctx)

	// 1. Validate AgenticServer exists and is active
	found, mcpServer, err := util.FindActiveAgenticServer(ctx, obj.Spec.BasePath)
	if err != nil {
		return err
	}
	obj.SetCondition(NewAgenticServerCondition(found))
	if !found {
		return handleAgenticServerNotFound(ctx, obj, mcpServer)
	}

	// 1b. Validate exposure scopes against AgenticServer's declared scopes
	if !validateExposureScopes(ctx, mcpServer, obj) {
		return nil
	}

	// 2. Check for competing exposures (oldest-wins)
	if blocked, checkErr := checkCompetingExposures(ctx, obj); checkErr != nil || blocked {
		return checkErr
	}

	// This exposure is active
	obj.Status.Active = true
	obj.SetCondition(NewAgenticExposureActiveCondition(true))

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

	crossZones, hasLocalSubs, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, obj.Spec.BasePath, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrap(err, "failed to find cross-zone MCP subscriptions")
	}

	var crossZoneLmsIssuers []string
	for _, subscriberZoneRef := range crossZones {
		subscriberZone, zoneErr := util.GetZone(ctx, subscriberZoneRef.K8s())
		if zoneErr != nil {
			return errors.Wrapf(zoneErr, "failed to get subscriber zone %q", subscriberZoneRef.Name)
		}

		// Collect LMS issuer so the real route trusts traffic forwarded by this proxy gateway
		if subscriberZone.Status.Links.LmsIssuer != "" {
			crossZoneLmsIssuers = append(crossZoneLmsIssuers, subscriberZone.Status.Links.LmsIssuer)
		}

		proxyRoute, routeErr := util.CreateAgenticProxyRoute(ctx, obj.Spec.BasePath, subscriberZone, zone)
		if routeErr != nil {
			return errors.Wrapf(routeErr, "failed to create MCP proxy Route for zone %q", subscriberZoneRef.Name)
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *ctypes.ObjectRefFromObject(proxyRoute))
		logger.V(1).Info("MCP proxy Route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	// 6. Resolve Telecontext Application for auto-access (TeleMCP variant)
	telecontextInfo, crossZoneLmsIssuers, err := h.resolveTelecontext(ctx, obj, crossZones, crossZoneLmsIssuers, zone)
	if err != nil {
		return err
	}

	// 7. Create primary MCP route
	isProxyTarget := len(obj.Status.ProxyRoutes) > 0
	telecontextConsumer := ""
	if telecontextInfo != nil {
		telecontextConsumer = telecontextInfo.ConsumerName
	}
	route, err := util.CreateAgenticRoute(ctx, obj, zone, hasLocalSubs, isProxyTarget, telecontextConsumer, crossZoneLmsIssuers)
	if err != nil {
		return errors.Wrap(err, "failed to create MCP Route")
	}
	obj.Status.Route = ctypes.ObjectRefFromObject(route)
	logger.V(1).Info("MCP Route created/updated", "route", route.Name)

	// 8. Cleanup stale routes
	deleted, err := util.CleanupOldAgenticRoutes(ctx, obj.Spec.BasePath)
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

	obj.SetCondition(condition.NewReadyCondition("AgenticExposureProvisioned",
		"AgenticExposure has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"AgenticExposure has been provisioned"))

	return nil
}

// resolveTelecontext handles the TELECONTEXTMCP variant: resolves the Telecontext Application,
// creates a proxy route on the Telecontext zone if needed, and returns the resolved info.
func (h *AgenticExposureHandler) resolveTelecontext(
	ctx context.Context,
	obj *agenticv1.AgenticExposure,
	crossZones []ctypes.ObjectRef,
	crossZoneLmsIssuers []string,
	providerZone *adminv1.Zone,
) (*util.TelecontextInfo, []string, error) {
	if !obj.Spec.Variant.IsTelecontextVariant() {
		return nil, crossZoneLmsIssuers, nil
	}

	logger := log.FromContext(ctx)

	if h.Config.TelecontextApplicationID == "" {
		return nil, nil, errors.New("TELECONTEXTMCP variant requires telecontext application ID to be configured")
	}

	info, err := util.ResolveTelecontextApplication(ctx, h.Config)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to resolve Telecontext Application")
	}

	proxyRef, lmsIssuer, proxyErr := ensureTelecontextProxyRoute(ctx, obj, info, crossZones, providerZone)
	if proxyErr != nil {
		return nil, nil, proxyErr
	}
	if proxyRef != nil {
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *proxyRef)
		logger.V(1).Info("MCP proxy Route created/updated for Telecontext zone", "zone", info.Zone.Name)
	}
	if lmsIssuer != "" {
		crossZoneLmsIssuers = append(crossZoneLmsIssuers, lmsIssuer)
	}

	return info, crossZoneLmsIssuers, nil
}

// checkCompetingExposures verifies that no other active AgenticExposure exists for the same basePath.
// Returns (true, nil) if this exposure is blocked by an older one, (false, nil) to continue, or (false, err) on failure.
func checkCompetingExposures(ctx context.Context, obj *agenticv1.AgenticExposure) (bool, error) {
	existingExposures, err := util.FindAgenticExposures(ctx, obj.Spec.BasePath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list AgenticExposures for basePath %q", obj.Spec.BasePath)
	}
	existingFound, existingExposure, err := util.FindActiveAgenticExposure(existingExposures)
	if err != nil {
		return false, errors.Wrapf(err, "failed to find active AgenticExposure for basePath %q", obj.Spec.BasePath)
	}

	if existingFound && existingExposure.UID != obj.UID {
		obj.Status.Active = false
		obj.SetCondition(NewAgenticExposureActiveCondition(false))
		msg := fmt.Sprintf("BasePath %q is already exposed by team %q.", obj.Spec.BasePath, existingExposure.Spec.Provider.Namespace)
		obj.SetCondition(condition.NewNotReadyCondition("AgenticExposureAlreadyExists", msg))
		obj.SetCondition(condition.NewBlockedCondition(msg + " AgenticExposure will be automatically processed when the existing one is deleted"))
		return true, nil
	}

	return false, nil
}

// handleAgenticServerNotFound handles the case where no active AgenticServer was found.
// It checks for case-only mismatches, cleans up stale routes, and sets blocking conditions.
func handleAgenticServerNotFound(ctx context.Context, obj *agenticv1.AgenticExposure, mcpServer *agenticv1.AgenticServer) error {
	if mcpServer != nil {
		msg := fmt.Sprintf("AgenticServer is registered but the case does not match (got=%q, found=%q). "+
			"Please resolve the conflict by changing the BasePath of either the AgenticServer or the AgenticExposure.",
			obj.Spec.BasePath, mcpServer.Spec.BasePath)
		obj.SetCondition(condition.NewNotReadyCondition("AgenticServerCaseConflict", msg))
		obj.SetCondition(condition.NewBlockedCondition(msg))
		return nil
	}

	if obj.Status.Route != nil {
		if cleanupErr := util.DeleteRouteIfExists(ctx, obj.Status.Route); cleanupErr != nil {
			return errors.Wrap(cleanupErr, "failed to cleanup Route after AgenticServer not found")
		}
	}
	for i := range obj.Status.ProxyRoutes {
		if cleanupErr := util.DeleteRouteIfExists(ctx, &obj.Status.ProxyRoutes[i]); cleanupErr != nil {
			return errors.Wrapf(cleanupErr, "failed to cleanup proxy Route after AgenticServer not found")
		}
	}

	obj.SetCondition(condition.NewNotReadyCondition("AgenticServerNotFound",
		"No active AgenticServer found for basePath "+obj.Spec.BasePath))
	obj.SetCondition(condition.NewBlockedCondition(
		"AgenticServer " + obj.Spec.BasePath + " does not exist or is not active. " +
			"AgenticExposure will be automatically processed when the AgenticServer is registered"))
	return nil
}

// validateExposureScopes checks that the M2M scopes in the AgenticExposure are a valid subset of the AgenticServer's scopes.
// It sets blocking conditions on the exposure and returns false if processing should stop.
func validateExposureScopes(_ context.Context, mcpServer *agenticv1.AgenticServer, obj *agenticv1.AgenticExposure) bool {
	if !obj.HasM2M() || obj.Spec.Security.M2M.Scopes == nil || obj.HasExternalIdp() {
		return true
	}
	if len(mcpServer.Spec.Oauth2Scopes) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "AgenticServer does not define any OAuth2 scopes"))
		obj.SetCondition(condition.NewBlockedCondition("AgenticServer does not define any OAuth2 scopes. AgenticExposure will be automatically processed, if the AgenticServer will be updated with scopes"))
		return false
	}
	scopesExist, invalidScopes := util.IsSubsetOfScopes(mcpServer.Spec.Oauth2Scopes, obj.Spec.Security.M2M.Scopes)
	if !scopesExist {
		message := fmt.Sprintf("Some defined scopes are not available. Available scopes: %q. Unsupported scopes: %q",
			strings.Join(mcpServer.Spec.Oauth2Scopes, ", "),
			strings.Join(invalidScopes, ", "),
		)
		obj.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes defined in AgenticExposure are not defined in the AgenticServer"))
		obj.SetCondition(condition.NewBlockedCondition(message))
		return false
	}
	return true
}

// ensureTelecontextProxyRoute creates a proxy route on the Telecontext Application's zone
// if it differs from the exposure zone and is not already covered by subscription-based cross zones.
// Returns the proxy route ObjectRef (nil if not needed), the LMS issuer to trust, and any error.
func ensureTelecontextProxyRoute(
	ctx context.Context,
	obj *agenticv1.AgenticExposure,
	info *util.TelecontextInfo,
	crossZones []ctypes.ObjectRef,
	providerZone *adminv1.Zone,
) (*ctypes.ObjectRef, string, error) {
	if info.Zone.Name == obj.Spec.Zone.Name {
		return nil, "", nil
	}

	for _, z := range crossZones {
		if z.Name == info.Zone.Name {
			return nil, "", nil
		}
	}

	telecontextZone, err := util.GetZone(ctx, info.Zone.K8s())
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to get Telecontext zone %q", info.Zone.Name)
	}

	proxyRoute, err := util.CreateAgenticProxyRoute(ctx, obj.Spec.BasePath, telecontextZone, providerZone)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to create MCP proxy Route for Telecontext zone %q", info.Zone.Name)
	}

	return ctypes.ObjectRefFromObject(proxyRoute), telecontextZone.Status.Links.LmsIssuer, nil
}

func (h *AgenticExposureHandler) Delete(ctx context.Context, obj *agenticv1.AgenticExposure) error {
	logger := log.FromContext(ctx)

	// Check if another AgenticExposure exists for the same basePath.
	otherExists, err := util.AnyOtherAgenticExposureExists(ctx, obj.Spec.BasePath, obj.UID)
	if err != nil {
		return errors.Wrap(err, "failed to check for other AgenticExposures")
	}

	if otherExists {
		logger.Info("Skipping Route deletion — another AgenticExposure exists for this basePath",
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
