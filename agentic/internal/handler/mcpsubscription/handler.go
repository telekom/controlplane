// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpsubscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/util"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

var _ handler.Handler[*agenticv1.McpSubscription] = &McpSubscriptionHandler{}

type McpSubscriptionHandler struct{}

//nolint:gocyclo // reconciler with sequential validation, approval, and provisioning steps
func (h *McpSubscriptionHandler) CreateOrUpdate(ctx context.Context, obj *agenticv1.McpSubscription) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// 1. Validate McpServer exists and is active
	found, mcpServer, findErr := util.FindActiveMcpServer(ctx, obj.Spec.BasePath)
	if findErr != nil {
		return findErr
	}
	if !found {
		obj.SetCondition(condition.NewNotReadyCondition("McpServerNotFound",
			"No active McpServer found for basePath "+obj.Spec.BasePath))
		obj.SetCondition(condition.NewBlockedCondition(
			"McpServer " + obj.Spec.BasePath + " does not exist or is not active. " +
				"McpSubscription will be automatically processed when the McpServer is registered"))
		return nil
	}

	// 1b. Validate subscription scopes against McpServer's declared scopes
	if !validateSubscriptionScopes(mcpServer, obj) {
		return nil
	}

	// 2. Find active McpExposure
	exposures, err := util.FindMcpExposures(ctx, obj.Spec.BasePath)
	if err != nil {
		return err
	}
	exposureFound, exposure, err := util.FindActiveMcpExposure(exposures)
	if err != nil {
		return errors.Wrapf(err, "failed to find active McpExposure for basePath %q", obj.Spec.BasePath)
	}
	if !exposureFound {
		obj.SetCondition(condition.NewNotReadyCondition("McpExposureNotFound",
			"No active McpExposure found for basePath "+obj.Spec.BasePath))
		obj.SetCondition(condition.NewBlockedCondition(
			"McpExposure for " + obj.Spec.BasePath + " does not exist or is not active. " +
				"McpSubscription will be automatically processed when the McpExposure is registered"))
		return nil
	}
	if err = condition.EnsureReady(exposure); err != nil {
		obj.SetCondition(condition.NewNotReadyCondition("McpExposureNotReady",
			fmt.Sprintf("McpExposure %q is not ready", exposure.Name)))
		obj.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("McpExposure %q is not ready. McpSubscription will be automatically processed when the McpExposure is ready", exposure.Name)))
		return nil
	}

	// 3. Validate subscriber zone supports AI Gateway
	subscriberZone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return err
	}
	if !subscriberZone.IsFeatureEnabled(adminv1.FeatureAiGateway) {
		obj.SetCondition(condition.NewNotReadyCondition("AiGatewayNotSupported",
			"Subscriber zone "+subscriberZone.Name+" does not support the AI Gateway feature"))
		return ctrlerrors.BlockedErrorf("subscriber zone %q does not support the AI Gateway feature", subscriberZone.Name)
	}

	// 4. Validate visibility rules
	valid := validateVisibility(exposure, obj, subscriberZone)
	if !valid {
		obj.SetCondition(condition.NewNotReadyCondition("VisibilityConstraintViolation",
			"McpExposure and McpSubscription visibility combination is not allowed"))
		return ctrlerrors.BlockedErrorf("McpSubscription is blocked. Subscriptions from zone %q are not allowed due to exposure visibility constraints", obj.Spec.Zone.Name)
	}

	// 5. Get requestor application
	requestorApp, err := util.GetApplication(ctx, obj.Spec.Requestor.Application)
	if err != nil {
		return err
	}

	// 6. Get provider application
	providerApp, err := util.GetApplication(ctx, exposure.Spec.Provider)
	if err != nil {
		return errors.Wrapf(err, "unable to get application from McpExposure provider %q", exposure.Spec.Provider.Name)
	}

	// 7. Build and evaluate approval
	requester := &approvalapi.Requester{
		TeamName:       requestorApp.Spec.Team,
		TeamEmail:      requestorApp.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(requestorApp, c.Scheme()),
		Reason: fmt.Sprintf("Team %s requested MCP subscription to %s from zone %s",
			requestorApp.Spec.Team, obj.Spec.BasePath, obj.Spec.Zone.Name),
	}

	properties := map[string]any{
		"mcpBasePath": obj.Spec.BasePath,
	}
	if err = requester.SetProperties(properties); err != nil {
		return errors.Wrapf(err, "unable to set approvalRequest properties for McpSubscription %q", obj.Name)
	}

	decider := &approvalapi.Decider{
		TeamName:       providerApp.Spec.Team,
		TeamEmail:      providerApp.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(providerApp, c.Scheme()),
	}

	approvalBuilder := builder.NewApprovalBuilder(c, obj)
	approvalBuilder.WithAction("subscribe")
	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithDecider(decider)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(exposure.Spec.Approval.Strategy))

	if len(exposure.Spec.Approval.TrustedTeams) > 0 {
		approvalBuilder.WithTrustedRequesters(exposure.Spec.Approval.TrustedTeams)
	}

	res, err := approvalBuilder.Build(ctx)
	if err != nil {
		return err
	}
	obj.Status.ApprovalRequest = types.ObjectRefFromObject(approvalBuilder.GetApprovalRequest())
	obj.Status.Approval = types.ObjectRefFromObject(approvalBuilder.GetApproval())

	switch res {
	case builder.ApprovalResultRequestDenied:
		logger.Info("ApprovalRequest was denied")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalRequestDenied", "ApprovalRequest has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return nil

	case builder.ApprovalResultPending:
		logger.Info("Approval is pending — waiting for approval")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Waiting for approval decision"))
		obj.SetCondition(condition.NewBlockedCondition("Waiting for approval decision"))
		return nil

	case builder.ApprovalResultDenied:
		logger.Info("Approval was denied — cleaning up ConsumeRoute")
		obj.SetCondition(condition.NewNotReadyCondition("ApprovalDenied", "Approval has been denied"))
		obj.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))

		deleted, cleanupErr := c.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(obj))
		if cleanupErr != nil {
			return errors.Wrapf(cleanupErr, "unable to cleanup ConsumeRoute for McpSubscription %q", obj.Name)
		}
		if deleted > 0 {
			logger.Info("Cleaned up ConsumeRoute resources", "deleted", deleted)
		}
		return nil

	case builder.ApprovalResultGranted:
		logger.Info("Approval is granted — continuing with provisioning")

	default:
		return errors.Errorf("unknown approval-builder result %q", res)
	}

	// 8. Provision ConsumeRoute
	routeRef, err := resolveRouteRef(ctx, obj, exposure, subscriberZone)
	if err != nil {
		return errors.Wrap(err, "failed to resolve route reference for ConsumeRoute")
	}
	if routeRef == nil {
		// Blocking condition already set by resolveRouteRef
		return nil
	}

	consumeRoute, err := h.createConsumeRoute(ctx, obj, *routeRef, requestorApp)
	if err != nil {
		return errors.Wrap(err, "failed to create ConsumeRoute")
	}
	obj.Status.ConsumeRoute = types.ObjectRefFromObject(consumeRoute)
	logger.V(1).Info("ConsumeRoute created/updated", "consumeRoute", consumeRoute.Name)

	// 9. Set final conditions
	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady",
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	obj.SetCondition(condition.NewReadyCondition("McpSubscriptionProvisioned",
		"McpSubscription has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition(
		"McpSubscription has been provisioned"))

	return nil
}

func (h *McpSubscriptionHandler) Delete(ctx context.Context, obj *agenticv1.McpSubscription) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	deleted, err := c.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(obj))
	if err != nil {
		return errors.Wrapf(err, "failed to cleanup ConsumeRoute for McpSubscription %q", obj.Name)
	}
	if deleted > 0 {
		logger.Info("Cleaned up ConsumeRoute resources", "deleted", deleted)
	}

	return nil
}

// resolveRouteRef determines which Route the ConsumeRoute should reference,
// based on whether the subscription is in the same zone as the exposure or cross-zone.
// Returns (nil, nil) if the route isn't ready yet — a blocking condition is set on obj.
func resolveRouteRef(
	ctx context.Context,
	obj *agenticv1.McpSubscription,
	exposure *agenticv1.McpExposure,
	subscriberZone *adminv1.Zone,
) (*types.ObjectRef, error) {
	logger := log.FromContext(ctx)

	if obj.Spec.Zone.Name == exposure.Spec.Zone.Name {
		// Same zone: reference the primary route directly
		if exposure.Status.Route == nil {
			obj.SetCondition(condition.NewNotReadyCondition("RouteNotReady",
				"McpExposure does not have a Route reference yet"))
			obj.SetCondition(condition.NewBlockedCondition("Waiting for McpExposure to create the route"))
			return nil, nil
		}
		logger.V(1).Info("Referencing primary route from McpExposure", "route", exposure.Status.Route.String())
		return exposure.Status.Route, nil
	}

	// Cross-zone: find the proxy route whose namespace matches the subscriber zone
	for i := range exposure.Status.ProxyRoutes {
		proxyRoute := &exposure.Status.ProxyRoutes[i]
		if proxyRoute.Namespace == subscriberZone.Status.Namespace {
			logger.V(1).Info("Referencing proxy route from McpExposure", "zone", obj.Spec.Zone.Name, "route", proxyRoute.String())
			return proxyRoute, nil
		}
	}

	obj.SetCondition(condition.NewNotReadyCondition("ProxyRouteNotReady",
		"McpExposure has not created a proxy route for zone "+obj.Spec.Zone.Name+" yet"))
	obj.SetCondition(condition.NewBlockedCondition(
		"Waiting for McpExposure to create the proxy route for zone " + obj.Spec.Zone.Name))
	return nil, nil
}

// createConsumeRoute creates a ConsumeRoute granting the subscriber access to the MCP route.
func (h *McpSubscriptionHandler) createConsumeRoute(
	ctx context.Context,
	obj *agenticv1.McpSubscription,
	routeRef types.ObjectRef,
	application *applicationv1.Application,
) (*gatewayapi.ConsumeRoute, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	consumeRouteName := routeRef.Name + "--" + labelutil.NormalizeNameValue(application.Status.ClientId)

	consumeRoute := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consumeRouteName,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, consumeRoute, c.Scheme()); err != nil {
			return errors.Wrapf(err, "failed to set owner reference on ConsumeRoute %q", consumeRouteName)
		}

		consumeRoute.Labels = map[string]string{
			config.DomainLabelKey:         "agentic",
			agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(obj.Spec.BasePath),
			config.BuildLabelKey("zone"):  obj.Spec.Zone.Name,
			config.BuildLabelKey("type"):  "mcp-subscription",
		}

		consumeRoute.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        routeRef,
			ConsumerName: application.Status.ClientId,
			Security:     util.MapSubscriberSecurityToGateway(obj.Spec.Security),
			Traffic:      util.MapSubscriberTrafficToGateway(&obj.Spec.Traffic),
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, consumeRoute, mutator); err != nil {
		return nil, errors.Wrapf(err, "failed to create or update ConsumeRoute %s/%s", consumeRoute.Namespace, consumeRoute.Name)
	}

	return consumeRoute, nil
}

// validateVisibility checks if the subscription is allowed given the exposure's visibility.
func validateVisibility(exposure *agenticv1.McpExposure, sub *agenticv1.McpSubscription, _ *adminv1.Zone) bool {
	switch exposure.Spec.Visibility {
	case agenticv1.VisibilityZone:
		// Only same-zone subscriptions
		return sub.Spec.Zone.Name == exposure.Spec.Zone.Name
	case agenticv1.VisibilityEnterprise:
		// Same enterprise (currently no cross-enterprise check, allow all)
		return true
	case agenticv1.VisibilityWorld:
		return true
	default:
		return false
	}
}

// validateSubscriptionScopes checks that the M2M scopes in the McpSubscription are a valid subset of the McpServer's scopes.
// It sets blocking conditions on the subscription and returns false if processing should stop.
func validateSubscriptionScopes(mcpServer *agenticv1.McpServer, obj *agenticv1.McpSubscription) bool {
	if !obj.HasM2M() || obj.Spec.Security.M2M.Scopes == nil {
		return true
	}
	if len(mcpServer.Spec.Oauth2Scopes) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "McpServer does not define any OAuth2 scopes"))
		obj.SetCondition(condition.NewBlockedCondition("McpServer does not define any OAuth2 scopes. McpSubscription will be automatically processed, if the McpServer will be updated with scopes"))
		return false
	}
	scopesExist, invalidScopes := util.IsSubsetOfScopes(mcpServer.Spec.Oauth2Scopes, obj.Spec.Security.M2M.Scopes)
	if !scopesExist {
		message := fmt.Sprintf("Some defined scopes are not available. Available scopes: %q. Unsupported scopes: %q",
			strings.Join(mcpServer.Spec.Oauth2Scopes, ", "),
			strings.Join(invalidScopes, ", "),
		)
		obj.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes defined in McpSubscription are not defined in the McpServer"))
		obj.SetCondition(condition.NewBlockedCondition(message))
		return false
	}
	return true
}
