// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	organizationapi "github.com/telekom/controlplane/organization/api/v1"
)

var _ handler.Handler[*apiapi.ApiExposure] = (*ApiExposureHandler)(nil)

type ApiExposureHandler struct{}

func (h *ApiExposureHandler) CreateOrUpdate(ctx context.Context, apiExp *apiapi.ApiExposure) error {
	logger := log.FromContext(ctx)

	// For an ApiExposure to be valid, two conditions need to be met:
	// 1. There must be a corresponding active Api.
	// 2. There must be no other active ApiExposure with the same base path.

	// 1. Check if corresponding active Api exists
	api, err := ApiMustExist(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to validate existence of Api for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}
	apiExp.SetCondition(NewApiCondition(apiExp, api != nil))

	if api == nil {
		// Api does not exist or is not active, conditions are already set in the function
		return nil
	}

	// check if there is already a different active apiExposure with same basepath
	err = ApiExposureMustNotAlreadyExist(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to validate uniqueness of ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	apiExp.SetCondition(NewApiExposureActiveCondition(apiExp, apiExp.Status.Active))
	if !apiExp.Status.Active {
		// its already exposed by another apiExposure, conditions are already set in the function
		return nil
	}

	// Core Validations are done, can continue

	apiExpApplication, err := util.GetApplicationFromLabel(ctx, apiExp)
	if err != nil {
		msg := fmt.Sprintf("Application for ApiExposure cannot be resolved: %v", err)
		apiExp.SetCondition(condition.NewNotReadyCondition("ApplicationResolutionFailed", msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
		return nil
	}

	if !validateApiCategoryPolicy(ctx, api, apiExpApplication, apiExp) {
		return nil
	}

	// Scopes: check if scopes exist and are a valid subset of the Api's scopes.
	if !validateExposureScopes(ctx, api, apiExp) {
		return nil
	}

	// --- Proxy Route Management ---
	crossZoneRefs, err := util.FindCrossZoneApiSubscriptionZones(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to find cross-zone subscription zones for apiExposure: %s", apiExp.Name)
	}

	// Reset proxy routes status
	apiExp.Status.ProxyRoutes = nil

	// Create proxy route for each unique subscriber zone
	for _, subscriberZoneRef := range crossZoneRefs {
		options := []util.CreateRouteOption{}

		// Pass provider failover zone if exists
		if apiExp.HasFailover() {
			options = append(options, util.WithFailoverZone(apiExp.Spec.Traffic.Failover.Zones[0]))
		}

		// Add service-level rate limits
		if apiExp.HasProviderRateLimit() {
			options = append(options, util.WithServiceRateLimit(apiExp.Spec.Traffic.RateLimit.Provider))
		}

		proxyRoute, createErr := util.CreateProxyRoute(ctx, subscriberZoneRef, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			contextutil.EnvFromContextOrDie(ctx),
			options...,
		)
		if createErr != nil {
			return errors.Wrapf(createErr, "failed to create proxy route for zone %s", subscriberZoneRef.Name)
		}
		apiExp.Status.ProxyRoutes = append(apiExp.Status.ProxyRoutes, *types.ObjectRefFromObject(proxyRoute))
		logger.V(1).Info("Proxy route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	// Create provider failover route if configured (must be before cleanup to prevent deletion)
	if apiExp.HasFailover() {
		failoverZone := apiExp.Spec.Traffic.Failover.Zones[0] // currently only one failover zone is supported
		failoverRoute, createErr := util.CreateProxyRoute(ctx, failoverZone, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			contextutil.EnvFromContextOrDie(ctx),
			util.WithFailoverUpstreams(apiExp.Spec.Upstreams...),
			util.WithFailoverSecurity(apiExp.Spec.Security),
		)
		if createErr != nil {
			return errors.Wrapf(createErr, "unable to create failover route for apiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
		}
		apiExp.Status.FailoverRoute = types.ObjectRefFromObject(failoverRoute)
	}

	// Cleanup stale proxy routes that were not touched in this reconciliation
	deleted, err := util.CleanupStaleProxyRoutes(ctx, apiExp.Spec.ApiBasePath)
	if err != nil {
		return errors.Wrap(err, "failed to cleanup stale proxy routes")
	}
	if deleted > 0 {
		logger.V(1).Info("Cleaned up stale proxy routes", "deleted", deleted)
	}

	// Create real route with proxy target flag if there are cross-zone subscribers
	// Check using HasCrossZoneSubscribers (approval-agnostic) for ACL, not just ProxyRoutes count
	// This ensures the gateway consumer is added even before subscriptions are approved,
	// preventing a brief window where proxy routes exist but can't access the real route
	hasCrossZoneSubs, err := HasCrossZoneSubscribers(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to check cross-zone subscribers for apiExposure: %s", apiExp.Name)
	}

	realRoute, err := util.CreateRealRoute(ctx, apiExp.Spec.Zone, apiExp, contextutil.EnvFromContextOrDie(ctx),
		util.WithProxyTarget(hasCrossZoneSubs),
	)
	if err != nil {
		return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	apiExp.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))
	apiExp.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiExp.Status.Route = types.ObjectRefFromObject(realRoute)
	logger.Info("✅ ApiExposure is processed")

	return nil
}

func (h *ApiExposureHandler) Delete(ctx context.Context, obj *apiapi.ApiExposure) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Clean up proxy routes
	for i := range obj.Status.ProxyRoutes {
		ref := &obj.Status.ProxyRoutes[i]
		proxyRoute := &gatewayapi.Route{}
		err := scopedClient.Get(ctx, ref.K8s(), proxyRoute)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return errors.Wrapf(err, "failed to get proxy route %s", ref.String())
		}
		logger.Info("🧹 Deleting proxy route of exposure", "route", ref.String())
		err = scopedClient.Delete(ctx, proxyRoute)
		if err != nil {
			return errors.Wrapf(err, "failed to delete proxy route %s", ref.String())
		}
	}

	if obj.Status.Route != nil {

		route := &gatewayapi.Route{}
		err := scopedClient.Get(ctx, obj.Status.Route.K8s(), route)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrap(err, "failed to get route")
		}

		logger.Info("🧹 Deleting real route of exposure")
		err = scopedClient.Delete(ctx, route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}
		logger.Info("✅ Successfully deleted obsolete route")
	}

	if obj.Status.FailoverRoute != nil {
		failoverRoute := &gatewayapi.Route{}
		err := scopedClient.Get(ctx, obj.Status.FailoverRoute.K8s(), failoverRoute)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrap(err, "failed to get failover route")
		}
		logger.Info("🧹 Deleting failover proxy route of exposure")
		err = scopedClient.Delete(ctx, failoverRoute)
		if err != nil {
			return errors.Wrapf(err, "failed to delete failover route")
		}
		logger.Info("✅ Successfully deleted obsolete failover route")
	}

	return nil
}

// validateExposureScopes checks that the M2M scopes in apiExp are a valid subset of the Api's scopes.
// It sets blocking conditions on apiExp and returns false if processing should stop.
func validateExposureScopes(ctx context.Context, api *apiapi.Api, apiExp *apiapi.ApiExposure) bool {
	if !apiExp.HasM2M() || apiExp.Spec.Security.M2M.Scopes == nil || apiExp.HasExternalIdp() {
		return true
	}
	if len(api.Spec.Oauth2Scopes) == 0 {
		apiExp.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "Api does not define any Oauth2 scopes"))
		apiExp.SetCondition(condition.NewBlockedCondition("Api does not define any Oauth2 scopes. ApiExposure will be automatically processed, if the API will be updated with scopes"))
		return false
	}
	scopesExist, invalidScopes := util.IsSubsetOfScopes(api.Spec.Oauth2Scopes, apiExp.Spec.Security.M2M.Scopes)
	if !scopesExist {
		message := fmt.Sprintf("Some defined scopes are not available. Available scopes: %q. Unsupported scopes: %q",
			strings.Join(api.Spec.Oauth2Scopes, ", "),
			strings.Join(invalidScopes, ", "),
		)
		apiExp.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes which are defined in ApiExposure are not defined in the ApiSpecification"))
		apiExp.SetCondition(condition.NewBlockedCondition(message))
		return false
	}
	log.FromContext(ctx).V(1).Info("✅ Scopes are valid and exist")
	return true
}

func validateApiCategoryPolicy(ctx context.Context, api *apiapi.Api, application *applicationapi.Application, apiExp *apiapi.ApiExposure) bool {
	team, err := organizationapi.FindTeamForObject(ctx, application)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.FromContext(ctx).V(1).Info("Skipping ApiCategory policy validation because team was not found")
			return true
		}
		msg := util.BuildApiCategoryPolicyResolutionMessage(api.Spec.Category, err)
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
		return false
	}

	apiCategory, err := util.ResolveActiveApiCategoryForApi(ctx, api)
	if err != nil {
		msg := util.BuildApiCategoryPolicyResolutionMessage(api.Spec.Category, err)
		var be ctrlerrors.BlockedError
		var re ctrlerrors.RetryableError

		if errors.As(err, &be) {
			log.FromContext(ctx).V(1).Info("ApiCategory policy validation blocked", "reason", err.Error())
			apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
			apiExp.SetCondition(condition.NewBlockedCondition(msg))
		} else if errors.As(err, &re) {
			log.FromContext(ctx).V(1).Info("ApiCategory policy validation retryable", "reason", err.Error())
			apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
		} else {
			log.FromContext(ctx).V(1).Info("ApiCategory policy validation failed", "reason", err.Error())
			apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
			apiExp.SetCondition(condition.NewBlockedCondition(msg))
		}
		return false
	}
	if apiCategory == nil {
		log.FromContext(ctx).V(1).Info("Skipping ApiCategory policy validation because no ApiCategories exist")
		return true
	}

	teamCategory := string(team.Spec.Category)
	if !apiCategory.IsAllowedForTeamCategory(teamCategory) {
		msg := util.BuildApiCategoryExposureDeniedMessage(teamCategory, apiCategory.Spec.LabelValue)
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryTeamCategoryNotAllowedReason, msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
		return false
	}

	return true
}
