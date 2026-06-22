// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
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

	// Scopes: check if scopes exist and are a valid subset of the Api's scopes.
	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows exposure of api category
	if !validateExposureScopes(ctx, api, apiExp) {
		return nil
	}

	// --- Proxy Route Management ---
	crossZoneRefs, err := util.FindCrossZoneApiSubscriptionZones(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to find cross-zone subscription zones for apiExposure: %s", apiExp.Name)
	}

	// Resolve the realm name (= environment name) for route security
	realmName := adminv1.RealmNameFromContext(ctx)

	// Reset proxy routes status
	apiExp.Status.ProxyRoutes = nil

	// Collect LMS issuers from cross-zone subscriber zones for the real route's TrustedIssuers.
	// The real route must trust LMS tokens from proxy gateways that forward traffic to it.
	var crossZoneLmsIssuers []string

	// Create proxy route for each unique subscriber zone
	for _, subscriberZoneRef := range crossZoneRefs {
		options := []util.CreateRouteOption{
			util.WithRealmName(realmName),
		}

		// Pass provider failover zone if exists
		if apiExp.HasFailover() {
			options = append(options, util.WithFailoverZone(apiExp.Spec.Traffic.Failover.Zones[0]))
		}

		// Add service-level rate limits
		if apiExp.HasProviderRateLimit() {
			options = append(options, util.WithServiceRateLimit(apiExp.Spec.Traffic.RateLimit.Provider))
		}

		proxyRoute, err := util.CreateProxyRoute(ctx, subscriberZoneRef, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			options...,
		)
		if createErr != nil {
			return errors.Wrapf(createErr, "failed to create proxy route for zone %s", subscriberZoneRef.Name)
		}
		apiExp.Status.ProxyRoutes = append(apiExp.Status.ProxyRoutes, *types.ObjectRefFromObject(proxyRoute))
		log.V(1).Info("Proxy route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)

		// Collect the subscriber zone's LMS issuer for the real route
		_, subscriberZone, zErr := util.GetDefaultPresetForZone(ctx, subscriberZoneRef)
		if zErr == nil && subscriberZone.Status.Links.LmsIssuer != "" {
			crossZoneLmsIssuers = append(crossZoneLmsIssuers, subscriberZone.Status.Links.LmsIssuer)
		}
	}

	// Create provider failover route if configured (must be before cleanup to prevent deletion)
	if apiExp.HasFailover() {
		failoverZone := apiExp.Spec.Traffic.Failover.Zones[0] // currently only one failover zone is supported
		failoverRoute, err := util.CreateProxyRoute(ctx, failoverZone, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			util.WithFailoverUpstreams(apiExp.Spec.Upstreams...),
			util.WithFailoverSecurity(apiExp.Spec.Security),
			util.WithTrustedIssuers(crossZoneLmsIssuers),
			util.WithRealmName(realmName),
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

	// --- Real Route ---
	// Build TrustedIssuers for the real route:
	// - IDP issuer (for local subscriber token validation) if local subscribers exist
	// - LMS issuers from cross-zone subscriber zones (for mesh token validation)
	hasCrossZoneSubs, err := HasCrossZoneSubscribers(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to check cross-zone subscribers for apiExposure: %s", apiExp.Name)
	}

	hasLocalSubs, err := HasLocalSubscribers(ctx, apiExp)
	if err != nil {
		return errors.Wrapf(err, "failed to check local subscribers for apiExposure: %s", apiExp.Name)
	}

	// Get exposure zone to read its IDP issuer
	_, exposureZone, err := util.GetDefaultPresetForZone(ctx, apiExp.Spec.Zone)
	if err != nil {
		return errors.Wrapf(err, "failed to get exposure zone %s", apiExp.Spec.Zone.Name)
	}

	var realRouteTrustedIssuers []string
	if hasLocalSubs && exposureZone.Status.Links.Issuer != "" {
		realRouteTrustedIssuers = append(realRouteTrustedIssuers, exposureZone.Status.Links.Issuer)
	}
	realRouteTrustedIssuers = append(realRouteTrustedIssuers, crossZoneLmsIssuers...)

	realRoute, err := util.CreateRealRoute(ctx, apiExp.Spec.Zone, apiExp,
		util.WithProxyTarget(hasCrossZoneSubs),
		util.WithTrustedIssuers(realRouteTrustedIssuers),
		util.WithRealmName(realmName),
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
