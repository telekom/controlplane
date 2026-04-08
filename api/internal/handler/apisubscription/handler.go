// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apisubscription/remote"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
)

var _ handler.Handler[*apiapi.ApiSubscription] = (*ApiSubscriptionHandler)(nil)

type ApiSubscriptionHandler struct{}

func (h *ApiSubscriptionHandler) CreateOrUpdate(ctx context.Context, apiSub *apiapi.ApiSubscription) error {
	log := log.FromContext(ctx)

	scopedClient := cclient.ClientFromContextOrDie(ctx)

	// Remote ApiSubscription handling
	if remote.IsRemoteApiSubscription(apiSub) {
		log.Info("ApiSubscription is remote")
		return remote.HandleRemoteApiSubscription(ctx, apiSub)
	}

	// Local ApiSubscription handling

	// For an ApiSubscription to be valid, several conditions need to be met:
	// 1. There must be a corresponding active Api.
	// 2. There must be a corresponding active ApiExposure.
	// 3. The ApiExposure visibility must be valid for the ApiSubscription zone.
	// 4. The Application must exist and be ready.
	// 5. Approval must be granted.

	// 1. Check if corresponding active Api exists
	api, err := ApiMustExist(ctx, apiSub)
	if err != nil {
		return err
	}
	apiSub.SetCondition(NewApiCondition(apiSub, api != nil))
	if api == nil {
		// Api does not exist or is not active, conditions are already set in the function
		return nil
	}

	// 2. Check if corresponding active ApiExposure exists
	apiExposure, err := ApiExposureMustExist(ctx, apiSub)
	if err != nil {
		return err
	}
	apiSub.SetCondition(NewApiExposureCondition(apiSub, apiExposure != nil))
	if apiExposure == nil {
		// ApiExposure does not exist or is not active, conditions are already set in the function
		return nil
	}

	// validate if basepathes of the api and apiexposure are really equal
	if api.Spec.BasePath != apiSub.Spec.ApiBasePath {
		// This should never happen as both Api and ApiExposure are looked up by the same basepath
		return errors.Wrapf(err, "Subscriptions basePath: %s does not match the APIs basepath: %s",
			apiSub.Spec.ApiBasePath, api.Spec.BasePath)
	}

	// 3. Validate ApiExposure visibility for ApiSubscription zone
	valid, err := ApiVisibilityMustBeValid(ctx, apiExposure, apiSub)
	if err != nil {
		return err
	}
	apiSub.SetCondition(NewVisibilityAllowedCondition(apiSub, string(apiExposure.Spec.Visibility), valid))
	if !valid {
		apiSub.SetCondition(condition.NewNotReadyCondition("VisibilityConstraintViolation", "ApiExposure and ApiSubscription visibility combination is not allowed"))
		return ctrlerrors.BlockedErrorf("ApiSubscription is blocked. Subscriptions from zone %q are not allowed due to exposure visiblity constraints", apiSub.Spec.Zone.GetName())
	}

	// 4. Check if Application exists and is ready
	apiSubApplication, err := util.GetApplication(ctx, apiSub.Spec.Requestor.Application)
	if err != nil {
		return err
	}

	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows subscription of api category

	// 5. Manage Approval process

	requester := &approvalapi.Requester{
		TeamName:       apiSubApplication.Spec.Team,
		TeamEmail:      apiSubApplication.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(apiSubApplication, scopedClient.Scheme()),
		Reason:         fmt.Sprintf("Team %s requested access to your API %s from zone %s", apiSubApplication.Spec.Team, api.Name, apiSub.Spec.Zone.Name),
	}
	properties := map[string]any{
		"basePath": apiSub.Spec.ApiBasePath,
	}

	// Scopes
	// check if scopes exist and scopes are subset from api
	if apiSub.HasM2M() {
		if apiSub.Spec.Security.M2M.Scopes != nil {
			if len(api.Spec.Oauth2Scopes) == 0 {
				apiSub.SetCondition(NewScopesAllowedCondition(apiSub, nil, false))
				apiSub.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "Api does not define any Oauth2 scopes"))
				apiSub.SetCondition(condition.NewBlockedCondition("Api does not define any Oauth2 scopes. ApiSubscription will be automatically processed, if the API will be updated with scopes"))
				return nil
			} else {
				scopesExist, invalidScopes := util.IsSubsetOfScopes(api.Spec.Oauth2Scopes, apiSub.Spec.Security.M2M.Scopes)
				if !scopesExist {
					var message = fmt.Sprintf("Some defined scopes are not available. Available scopes: \"%s\". Unsupported scopes: \"%s\"",
						strings.Join(api.Spec.Oauth2Scopes, ", "),
						strings.Join(invalidScopes, ", "),
					)
					apiSub.SetCondition(NewScopesAllowedCondition(apiSub, invalidScopes, false))
					apiSub.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes which are defined in ApiSubscription are not defined in the ApiSpecification"))
					apiSub.SetCondition(condition.NewBlockedCondition(message))
					return nil
				}
			}
		}

		log.V(1).Info("✅ Scopes are valid and exist")
		apiSub.SetCondition(NewScopesAllowedCondition(apiSub, apiSub.Spec.Security.M2M.Scopes, true))
		properties["scopes"] = apiSub.Spec.Security.M2M.Scopes
	}
	err = requester.SetProperties(properties)
	if err != nil {
		return errors.Wrapf(err, "unable to set approvalRequest properties for apiSubscription: %q in namespace: %q",
			apiSub.Name, apiSub.Namespace)
	}

	// create the approval decider - entity that owns the requested object
	apiExpApplication, err := util.GetApplicationFromLabel(ctx, apiExposure)
	if err != nil {
		return errors.Wrapf(err, "unable to get application from apiExposure label: %q while handling apiSubscription %q", apiExposure.Name, apiSub.Name)
	}
	decider := &approvalapi.Decider{
		TeamName:       apiExpApplication.Spec.Team,
		TeamEmail:      apiExpApplication.Spec.TeamEmail,
		ApplicationRef: types.TypedObjectRefFromObject(apiExpApplication, scopedClient.Scheme()),
	}

	approvalBuilder := builder.NewApprovalBuilder(scopedClient, apiSub)
	approvalBuilder.WithAction("subscribe")
	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithDecider(decider)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(apiExposure.Spec.Approval.Strategy))

	if len(apiExposure.Spec.Approval.TrustedTeams) > 0 {
		approvalBuilder.WithTrustedRequesters(apiExposure.Spec.Approval.TrustedTeams)
	}

	res, err := approvalBuilder.Build(ctx)
	if err != nil {
		return err
	}
	apiSub.Status.ApprovalRequest = types.ObjectRefFromObject(approvalBuilder.GetApprovalRequest())
	apiSub.Status.Approval = types.ObjectRefFromObject(approvalBuilder.GetApproval())

	switch res {
	case builder.ApprovalResultRequestDenied:
		log.Info("🛑 ApprovalRequest was denied. In this case we will not touch child resources")
		apiSub.SetCondition(condition.NewNotReadyCondition("ApprovalRequestDenied", "ApprovalRequest has been denied"))
		apiSub.SetCondition(condition.NewDoneProcessingCondition("ApprovalRequest has been denied"))
		return nil
	case builder.ApprovalResultPending:
		log.Info("🫷 Approval is pending and we will wait for it")
		apiSub.SetCondition(condition.NewNotReadyCondition("ApprovalPending", "Approval has not been approved"))
		apiSub.SetCondition(condition.NewBlockedCondition("Approval has not been approved"))
		return nil
	case builder.ApprovalResultDenied:
		apiSub.SetCondition(condition.NewNotReadyCondition("ApprovalDenied", "Approval has been denied"))
		apiSub.SetCondition(condition.NewDoneProcessingCondition("Approval has been denied"))

		deleted, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return errors.Wrapf(err, "Unable to cleanup consume routes for ApiSubscription:  %q in namespace: %q",
				apiSub.Name, apiSub.Namespace)
		}
		log.Info("🧹 Approval was denied. Cleaning up Consumer of ApiSubscription", "deleted", deleted)

		err = util.CleanupProxyRoute(ctx, apiSub.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}
		log.Info("🧹 Approval was denied. Cleaning up ProxyRoute of ApiSubscription")
		return nil
	case builder.ApprovalResultGranted:
		log.Info("👌 Approval is granted and will continue with processing")
	default:
		return errors.Errorf("unknown approval-builder result %q", res)
	}

	// Construct reference to the route using ApiExposure.Status
	// The route reference depends on the subscription zone:
	// - Same zone as exposure: use the real route from ApiExposure.Status.Route
	// - Provider failover zone: use the failover route from ApiExposure.Status.FailoverRoute
	// - Cross-zone: find matching proxy route in ApiExposure.Status.ProxyRoutes

	sameZoneAsExposure := apiSub.Spec.Zone.Equals(&apiExposure.Spec.Zone)
	inProviderFailoverZone := apiExposure.HasFailover() && apiExposure.Spec.Traffic.Failover.ContainsZone(apiSub.Spec.Zone)

	var routeRef *types.ObjectRef

	if sameZoneAsExposure {
		// Same zone: reference the real route from ApiExposure status
		if apiExposure.Status.Route == nil {
			apiSub.SetCondition(condition.NewBlockedCondition("Waiting for ApiExposure to create the route"))
			return nil
		}
		routeRef = apiExposure.Status.Route
		log.V(1).Info("Referencing real route from ApiExposure", "route", routeRef.String())
	} else if inProviderFailoverZone {
		// Provider failover zone: reference the failover route from ApiExposure status
		if apiExposure.Status.FailoverRoute == nil {
			apiSub.SetCondition(condition.NewBlockedCondition("Waiting for ApiExposure to create the failover route"))
			return nil
		}
		routeRef = apiExposure.Status.FailoverRoute
		log.V(1).Info("Referencing provider failover route from ApiExposure", "route", routeRef.String())
	} else {
		// Cross-zone: find the proxy route for this subscriber zone in ApiExposure status
		// Get the subscription zone to find the correct proxy route by namespace
		zone, err := util.GetZone(ctx, scopedClient, apiSub.Spec.Zone.K8s())
		if err != nil {
			return errors.Wrapf(err, "failed to get subscription zone %s", apiSub.Spec.Zone.Name)
		}

		// Find the proxy route that matches our zone's namespace
		var found bool
		for _, proxyRoute := range apiExposure.Status.ProxyRoutes {
			if proxyRoute.Namespace == zone.Status.Namespace {
				routeRef = &proxyRoute
				found = true
				break
			}
		}

		if !found {
			apiSub.SetCondition(condition.NewBlockedCondition("Waiting for ApiExposure to create the proxy route for this zone"))
			return nil
		}
		log.V(1).Info("Referencing proxy route from ApiExposure", "zone", apiSub.Spec.Zone.Name, "route", routeRef.String())
	}

	apiSub.Status.Route = routeRef

	consumeRouteOptions := []util.CreateConsumeRouteOption{}

	if limits, ok := apiExposure.GetOverriddenSubscriberRateLimit(apiSubApplication.Status.ClientId); ok {
		consumeRouteOptions = append(consumeRouteOptions, util.WithConsumerRouteRateLimit(limits))
	} else if apiExposure.HasDefaultSubscriberRateLimit() {
		consumeRouteOptions = append(consumeRouteOptions, util.WithConsumerRouteRateLimit(apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default.Limits))
	}

	consumeRoute, err := util.CreateConsumeRoute(ctx, apiSub, apiSub.Spec.Zone, *routeRef, apiSubApplication.Status.ClientId, consumeRouteOptions...)
	if err != nil {
		return errors.Wrapf(err, "failed to create normal ConsumeRoute")
	}
	apiSub.Status.ConsumeRoute = types.ObjectRefFromObject(consumeRoute)

	// ----- Subscriber Failover -----
	// In the exposure-driven pattern, ApiExposure creates proxy routes in all failover zones.
	// ApiSubscription just references them and creates ConsumeRoutes.

	apiSub.Status.FailoverRoutes = []types.ObjectRef{}
	apiSub.Status.FailoverConsumeRoutes = []types.ObjectRef{}
	if apiSub.HasFailover() {
		for _, subFailoverZone := range apiSub.Spec.Traffic.Failover.Zones {
			// Construct reference to the failover route (created by ApiExposure)
			// Same logic as main route: check if it's in exposure zone or provider failover zone
			sameZoneAsExposure := subFailoverZone.Equals(&apiExposure.Spec.Zone)
			inProviderFailoverZone := apiExposure.HasFailover() && apiExposure.Spec.Traffic.Failover.ContainsZone(subFailoverZone)

			var failoverRouteRef types.ObjectRef
			if sameZoneAsExposure {
				// Reference the real route in exposure zone
				zone, err := util.GetZone(ctx, scopedClient, apiExposure.Spec.Zone.K8s())
				if err != nil {
					return errors.Wrapf(err, "failed to get exposure zone %s for failover", apiExposure.Spec.Zone.Name)
				}
				failoverRouteRef = types.ObjectRef{
					Name:      util.MakeRouteName(apiSub.Spec.ApiBasePath, contextutil.EnvFromContextOrDie(ctx)),
					Namespace: zone.Status.Namespace,
				}
				log.V(1).Info("Referencing real route in failover zone (same as exposure)", "zone", subFailoverZone.Name)
			} else if inProviderFailoverZone {
				// Reference the provider failover route
				failoverZone, err := util.GetZone(ctx, scopedClient, apiExposure.Spec.Traffic.Failover.Zones[0].K8s())
				if err != nil {
					return errors.Wrapf(err, "failed to get provider failover zone %s", apiExposure.Spec.Traffic.Failover.Zones[0].Name)
				}
				failoverRouteRef = types.ObjectRef{
					Name:      util.MakeRouteName(apiSub.Spec.ApiBasePath, contextutil.EnvFromContextOrDie(ctx)),
					Namespace: failoverZone.Status.Namespace,
				}
				log.V(1).Info("Referencing provider failover route", "zone", subFailoverZone.Name)
			} else {
				// Reference the proxy route created by ApiExposure in subscriber failover zone
				zone, err := util.GetZone(ctx, scopedClient, subFailoverZone.K8s())
				if err != nil {
					return errors.Wrapf(err, "failed to get subscriber failover zone %s", subFailoverZone.Name)
				}
				failoverRouteRef = types.ObjectRef{
					Name:      util.MakeRouteName(apiSub.Spec.ApiBasePath, contextutil.EnvFromContextOrDie(ctx)),
					Namespace: zone.Status.Namespace,
				}
				log.V(1).Info("Referencing proxy route created by ApiExposure for subscriber failover", "zone", subFailoverZone.Name)
			}

			apiSub.Status.FailoverRoutes = append(apiSub.Status.FailoverRoutes, failoverRouteRef)

			// Create ConsumeRoute for the failover route
			log.V(1).Info("Creating failover ConsumeRoute for zone", "zone", subFailoverZone.Name)
			consumeRoute, err = util.CreateConsumeRoute(ctx, apiSub, subFailoverZone, failoverRouteRef, apiSubApplication.Status.ClientId)
			if err != nil {
				return errors.Wrapf(err, "failed to create failover ConsumeRoute for zone %s", subFailoverZone.Name)
			}
			apiSub.Status.FailoverConsumeRoutes = append(apiSub.Status.FailoverConsumeRoutes, *types.ObjectRefFromObject(consumeRoute))
		}
	}

	// ---- Set Conditions ----
	apiSub.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiSub.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))

	log.Info("✅ Successfully processed ApiSubscription")
	return nil
}

func (h *ApiSubscriptionHandler) Delete(ctx context.Context, apiSub *apiapi.ApiSubscription) error {
	// Main proxy route cleanup is now handled by ApiExposure (exposure-driven pattern)
	// Only cleanup subscriber failover routes

	for _, failoverRoute := range apiSub.Status.FailoverRoutes {
		err := util.CleanupProxyRoute(ctx, &failoverRoute)
		if err != nil {
			return errors.Wrapf(err, "failed to delete failover route")
		}
	}
	return nil
}
