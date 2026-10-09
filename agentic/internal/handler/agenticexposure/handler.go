// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

var _ handler.Handler[*agenticv1.AgenticExposure] = &AgenticExposureHandler{}

type AgenticExposureHandler struct {
	Config *agenticconfig.AgenticConfig
}

func (h *AgenticExposureHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.AgenticExposure) error { //nolint:gocyclo // Reconciliation is a linear sequence of independently failing operations.
	logger := log.FromContext(ctx)

	// 1. Validate server (McpServer or AgentCard) exists and is active
	serverInfo, caseConflict, err := util.ServerMustExist(ctx, obj.Spec.BasePath)
	if err != nil {
		return err
	}
	if serverInfo == nil {
		obj.SetCondition(NewServerCondition(false))
		if caseConflict {
			// Case-conflict conditions already set by ServerMustExist
			return handleServerNotFoundCleanup(ctx, obj)
		}
		return handleServerNotFound(ctx, obj)
	}
	obj.SetCondition(NewServerCondition(true))

	// 1b. Validate exposure credentials and scopes
	if err = validateBasicWithScopesPolicy(obj); err != nil {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonValidationFailed, err.Error()))
		obj.SetCondition(condition.NewBlockedCondition(err.Error()))
		return nil
	}
	if !validateExposureScopes(ctx, serverInfo, obj) {
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
	if !zone.Spec.FeaturesSupported(adminv1.GatewayTypeAI) {
		obj.SetCondition(condition.NewNotReadyCondition("AiGatewayNotSupported",
			"Zone "+zone.Name+" does not support the AI Gateway feature"))
		return ctrlerrors.BlockedErrorf("zone %q does not support the AI Gateway feature", zone.Name)
	}

	// 5. Resolve the Telecontext Application before the status is reset
	var telecontextApp *applicationapi.Application
	if obj.Spec.Variant.IsTelecontextVariant() {
		if telecontextApp, err = h.getTelecontextApplication(ctx); err != nil {
			return errors.Wrap(err, "failed to resolve Telecontext Application")
		}
	}

	obj.Status.Route = nil
	obj.Status.ProxyRoutes = nil

	// 6. Handle cross-zone proxy routes for subscriber zones and the Telecontext zone
	crossZones, hasLocalSubs, err := util.FindCrossZoneAgenticSubscriptionZones(ctx, obj.Spec.BasePath, obj.Spec.Zone.Name)
	if err != nil {
		return errors.Wrap(err, "failed to find cross-zone MCP subscriptions")
	}

	proxyZones := slices.Clone(crossZones)
	if obj.Spec.Variant.IsTelecontextVariant() && telecontextApp.Spec.Zone.Name != obj.Spec.Zone.Name &&
		!slices.ContainsFunc(proxyZones, func(z ctypes.ObjectRef) bool { return z.Name == telecontextApp.Spec.Zone.Name }) {
		proxyZones = append(proxyZones, telecontextApp.Spec.Zone)
		// Keep the order stable, like FindCrossZoneAgenticSubscriptionZones, so the status does not change
		slices.SortFunc(proxyZones, func(a, b ctypes.ObjectRef) int {
			return strings.Compare(a.String(), b.String())
		})
	}

	var crossZoneLmsIssuers []string
	for _, proxyZoneRef := range proxyZones {
		proxyZone, zoneErr := util.GetZone(ctx, proxyZoneRef.K8s())
		if zoneErr != nil {
			return errors.Wrapf(zoneErr, "failed to get proxy zone %q", proxyZoneRef.Name)
		}

		// Collect LMS issuer so the real route trusts traffic forwarded by this proxy gateway
		proxyPreset, presetErr := proxyZone.Spec.SelectPreset(adminv1.GatewayTypeAI)
		if presetErr != nil {
			return ctrlerrors.BlockedErrorf("proxy zone %q has no AI Gateway preset: %v", proxyZone.Name, presetErr)
		}
		proxyPresetStatus, presetErr := proxyZone.Status.GetPreset(proxyPreset.Name)
		if presetErr != nil {
			return ctrlerrors.BlockedErrorf("proxy zone %q has no AI Gateway preset status: %v", proxyZone.Name, presetErr)
		}
		if proxyPresetStatus.Links.LmsIssuer != "" {
			crossZoneLmsIssuers = append(crossZoneLmsIssuers, proxyPresetStatus.Links.LmsIssuer)
		}

		// Telecontext calls the proxy route in its own zone
		var extraConsumers []string
		if obj.Spec.Variant.IsTelecontextVariant() && telecontextApp.Spec.Zone.Name == proxyZoneRef.Name {
			extraConsumers = append(extraConsumers, telecontextApp.Status.ClientId)
		}

		proxyRoute, routeErr := util.CreateAgenticProxyRoute(ctx, obj.Spec.BasePath, obj.Spec.Variant, proxyZone, zone,
			extraConsumers...)
		if routeErr != nil {
			return errors.Wrapf(routeErr, "failed to create MCP proxy Route for zone %q", proxyZoneRef.Name)
		}
		obj.Status.ProxyRoutes = append(obj.Status.ProxyRoutes, *ctypes.ObjectRefFromObject(proxyRoute))
		logger.V(1).Info("MCP proxy Route created/updated", "zone", proxyZoneRef.Name, "route", proxyRoute.Name)
	}

	// 7. Create primary MCP route
	isProxyTarget := len(obj.Status.ProxyRoutes) > 0
	telecontextConsumer := ""
	if obj.Spec.Variant.IsTelecontextVariant() && telecontextApp.Spec.Zone.Name == obj.Spec.Zone.Name {
		telecontextConsumer = telecontextApp.Status.ClientId
	}
	hasLocalCallers := hasLocalSubs || telecontextConsumer != ""
	route, err := util.CreateAgenticRoute(ctx, obj, zone, hasLocalCallers, isProxyTarget, telecontextConsumer, crossZoneLmsIssuers)
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

// handleServerNotFound handles the case where no active server was found.
// It cleans up stale routes and sets blocking conditions.
func handleServerNotFound(ctx context.Context, obj *agenticv1.AgenticExposure) error {
	if err := cleanupServerRoutes(ctx, obj); err != nil {
		return err
	}

	obj.SetCondition(condition.NewNotReadyCondition("ServerNotFound",
		"No active agentic server (McpServer, AgentCard) found for basePath "+obj.Spec.BasePath))
	obj.SetCondition(condition.NewBlockedCondition(
		"Agentic server for " + obj.Spec.BasePath + " does not exist or is not active. " +
			"AgenticExposure will be automatically processed when the specification is registered"))
	return nil
}

// handleServerNotFoundCleanup handles the case where a case-conflict was detected.
// ServerMustExist already reported the conflict via caseConflict=true; this sets conditions and cleans up routes.
func handleServerNotFoundCleanup(ctx context.Context, obj *agenticv1.AgenticExposure) error {
	if err := cleanupServerRoutes(ctx, obj); err != nil {
		return err
	}

	obj.SetCondition(condition.NewNotReadyCondition("CaseConflict",
		"Agentic server (McpServer, AgentCard) is registered but the basePath case does not match"))
	obj.SetCondition(condition.NewBlockedCondition(
		"Agentic server for " + obj.Spec.BasePath + " exists but with a different case. " +
			"Please resolve the conflict by changing the BasePath of either the specification or the exposure"))
	return nil
}

// cleanupServerRoutes removes routes when the server is not found or has a case conflict.
func cleanupServerRoutes(ctx context.Context, obj *agenticv1.AgenticExposure) error {
	if obj.Status.Route != nil {
		if cleanupErr := util.DeleteRouteIfExists(ctx, obj.Status.Route); cleanupErr != nil {
			return errors.Wrap(cleanupErr, "failed to cleanup Route after server not found")
		}
	}
	for i := range obj.Status.ProxyRoutes {
		if cleanupErr := util.DeleteRouteIfExists(ctx, &obj.Status.ProxyRoutes[i]); cleanupErr != nil {
			return errors.Wrapf(cleanupErr, "failed to cleanup proxy Route after server not found")
		}
	}
	return nil
}

// validateExposureScopes checks M2M scope membership unless the exposure has a non-empty external-IDP token endpoint.
// It sets blocking conditions on the exposure and returns false if processing should stop.
func validateExposureScopes(_ context.Context, server *util.ServerInfo, obj *agenticv1.AgenticExposure) bool {
	if !obj.HasM2M() || obj.Spec.Security.M2M.Scopes == nil || obj.HasExternalIdp() {
		return true
	}
	if len(server.Oauth2Scopes) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "Server does not define any OAuth2 scopes"))
		obj.SetCondition(condition.NewBlockedCondition("Server does not define any OAuth2 scopes. AgenticExposure will be automatically processed, if the server will be updated with scopes"))
		return false
	}
	scopesExist, invalidScopes := util.IsSubsetOfScopes(server.Oauth2Scopes, obj.Spec.Security.M2M.Scopes)
	if !scopesExist {
		message := fmt.Sprintf("Some defined scopes are not available. Available scopes: %q. Unsupported scopes: %q",
			strings.Join(server.Oauth2Scopes, ", "),
			strings.Join(invalidScopes, ", "),
		)
		obj.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes defined in AgenticExposure are not defined in the server"))
		obj.SetCondition(condition.NewBlockedCondition(message))
		return false
	}
	return true
}

func validateBasicWithScopesPolicy(obj *agenticv1.AgenticExposure) error {
	if !obj.HasExternalIdp() || len(obj.Spec.Security.M2M.Scopes) == 0 {
		return nil
	}
	idp := obj.Spec.Security.M2M.ExternalIDP
	if idp.Basic == nil || idp.GrantType == agenticv1.GrantTypePassword {
		return nil
	}
	return errors.New("Provider username/password with scopes requires an external IDP grant type \"password\"")
}

// getTelecontextApplication returns the configured Telecontext Application.
// The Application must be ready and have a client ID, which is its gateway consumer name.
func (h *AgenticExposureHandler) getTelecontextApplication(ctx context.Context) (*applicationapi.Application, error) {
	group, team, appName, err := h.Config.ParseTelecontextApplicationID()
	if err != nil {
		return nil, err
	}

	ref := ctypes.ObjectRef{
		Name:      appName,
		Namespace: contextutil.EnvFromContextOrDie(ctx) + "--" + group + "--" + team,
	}
	application, err := util.GetApplication(ctx, ref)
	if err != nil {
		return nil, err
	}
	if application.Status.ClientId == "" {
		return nil, ctrlerrors.BlockedErrorf("application %q has no client ID", ref.String())
	}

	return application, nil
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
