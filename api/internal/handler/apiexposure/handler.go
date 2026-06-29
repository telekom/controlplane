// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	stderrors "errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
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

	// --- Route Provisioning Pipeline ---

	// 1. Determine routing state (subscribers, flags, exposure zone)
	state, err := h.determineRoutingState(ctx, apiExp)
	if err != nil {
		return err
	}

	// 2. Create proxy routes (also collects consumer failover enrichment into state)
	if manageErr := h.manageProxyRoutes(ctx, apiExp, state); manageErr != nil {
		return manageErr
	}

	// 3. Create real route (uses enriched state)
	realRoute, err := h.createRealRoute(ctx, apiExp, state)
	if err != nil {
		return err
	}

	apiExp.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))
	apiExp.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiExp.Status.Route = types.ObjectRefFromObject(realRoute)
	logger.Info("✅ ApiExposure is processed")

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Route Provisioning Pipeline
// ──────────────────────────────────────────────────────────────────────────────
//
// The route provisioning pipeline runs in three steps:
//
//  1. determineRoutingState — fetches subscribers and zone data, derives flags.
//  2. manageProxyRoutes    — creates proxy routes, collects consumer failover enrichment.
//  3. createRealRoute      — creates the real route using the enriched state.
//
// All steps share a single routingState instance to avoid redundant API calls
// and make the data flow between steps explicit.

// routingState holds pre-computed data used across the route provisioning pipeline.
//
// The distinction between "consumer failover" and "provider failover" is important:
//   - Consumer failover (a.k.a. DTC/DNS failover): all proxy routes AND the real route are
//     enriched with additional hostnames and IDP issuers from ALL zones that have the
//     ConsumerFailover feature enabled. This allows external DNS to switch consumers
//     between zone gateways transparently.
//   - Provider failover: creates a secondary route in a backup zone with the provider's
//     upstreams, allowing traffic to be served from a different zone if the provider fails.
//     Managed separately via WithFailoverUpstreams/WithFailoverSecurity.
type routingState struct {
	// ──────────────────────────────────────────────────────────────────────────
	// Determined up front by determineRoutingState
	// ──────────────────────────────────────────────────────────────────────────

	// realmName is the environment/realm name used for token validation on all routes.
	realmName string

	// subscribers is the full list of approved, non-deleted ApiSubscriptions for this exposure.
	// Used to determine which zones need proxy routes and whether consumer failover is active.
	subscribers []*apiapi.ApiSubscription

	// hasCrossZoneSubs is true if at least one subscriber is in a different zone than the exposure.
	// Drives: real route gets GatewayConsumerName in DefaultConsumers (mesh-client access).
	hasCrossZoneSubs bool

	// hasLocalSubs is true if at least one subscriber is in the same zone as the exposure.
	// Drives: real route trusts the exposure zone's own IDP issuer (direct consumer access).
	hasLocalSubs bool

	// hasConsumerFailover is true if at least one subscriber has consumer failover configured.
	// When true, ALL proxy routes and the real route are enriched with failover hostnames/issuers.
	hasConsumerFailover bool

	// exposureZone is the Zone object where the API is exposed (provider zone).
	// Used to read the zone's IDP issuer and to identify which zone is "self" in the failover loop.
	exposureZone *adminv1.Zone

	// ──────────────────────────────────────────────────────────────────────────
	// Consumer failover enrichment — produced by manageProxyRoutes
	// Applied to ALL proxy routes AND the real route.
	// Collected from every zone that has the ConsumerFailover feature enabled
	// (including the exposure zone itself).
	// ──────────────────────────────────────────────────────────────────────────

	// consumerFailoverHosts are the hostnames from the ConsumerFailover gateway presets of all
	// eligible zones. Added as additional hostnames so that any zone's gateway can accept
	// traffic for any other zone's failover hostname after a DNS switch.
	consumerFailoverHosts []string

	// consumerFailoverPaths are the paths from the ConsumerFailover gateway presets of all
	// eligible zones. Added alongside consumerFailoverHosts.
	consumerFailoverPaths []string

	// consumerFailoverIssuers are the IDP issuers from all eligible zones.
	// Added as trusted issuers so that when a consumer fails over to a different zone's
	// gateway, the route can validate the consumer's home-zone IDP token directly.
	consumerFailoverIssuers []string

	// ──────────────────────────────────────────────────────────────────────────
	// Mesh trust — produced by manageProxyRoutes
	// Only for the real route. NOT related to consumer failover.
	// ──────────────────────────────────────────────────────────────────────────

	// crossZoneLmsIssuers are the LMS (Last-Mile-Security) issuers from all non-exposure
	// zones that have proxy routes. The real route must trust these because proxy gateways
	// in other zones stamp an LMS token before forwarding traffic to the provider zone.
	// This is a mesh concern, not a consumer failover concern.
	crossZoneLmsIssuers []string
}

// determineRoutingState fetches and pre-computes all data needed for route provisioning.
// This avoids redundant API calls across the provisioning pipeline.
func (h *ApiExposureHandler) determineRoutingState(ctx context.Context, apiExp *apiapi.ApiExposure) (*routingState, error) {
	state := &routingState{
		realmName: adminv1.RealmNameFromContext(ctx),
	}

	// Fetch all non-deleted subscribers for this exposure.
	subscribers, err := util.FindAllSubscribersForApiExposure(ctx, apiExp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find subscribers for apiExposure: %s", apiExp.Name)
	}
	state.subscribers = subscribers

	// Derive cross-zone and local subscriber flags from the fetched list.
	for _, sub := range subscribers {
		if sub.Spec.Zone.Equals(&apiExp.Spec.Zone) {
			state.hasLocalSubs = true
		} else {
			state.hasCrossZoneSubs = true
		}
		if state.hasLocalSubs && state.hasCrossZoneSubs {
			break // both flags set, no need to check further
		}
	}

	// Consumer failover is active if ANY subscriber has it configured.
	// When active, we enrich ALL routes (proxy + real) with failover hostnames and issuers.
	state.hasConsumerFailover = slices.ContainsFunc(subscribers, func(sub *apiapi.ApiSubscription) bool {
		return sub.HasFailover()
	})

	// Fetch the exposure zone — needed for IDP issuer (real route) and
	// to identify "self" in the consumer failover loop.
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	myZone, err := util.GetZone(ctx, scopedClient, apiExp.Spec.Zone.K8s())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get exposure zone %q for apiExposure: %s", apiExp.Spec.Zone.Name, apiExp.Name)
	}

	state.exposureZone = myZone

	return state, nil
}

// manageProxyRoutes creates proxy routes for each cross-zone subscriber zone and handles
// the provider failover (secondary) route. It also collects consumer failover enrichment
// data into the routingState for use by createRealRoute.
func (h *ApiExposureHandler) manageProxyRoutes(ctx context.Context, apiExp *apiapi.ApiExposure, state *routingState) error {
	logger := log.FromContext(ctx)

	// Build the set of zones that need a proxy route.
	// Starts with cross-zone subscriber zones, then extended by consumer failover zones.
	allRelevantZones := h.collectCrossZoneSubscriberZones(apiExp, state)

	// --- Consumer failover: collect enrichment data from all eligible zones ---
	if state.hasConsumerFailover {
		logger.Info("Consumer failover is enabled for at least one subscriber")

		var err error
		allRelevantZones, err = h.collectConsumerFailoverEnrichment(ctx, apiExp, state, allRelevantZones)
		if err != nil {
			return err
		}
	}

	// --- Collect LMS issuers from all cross-zone proxy route zones ---
	// Proxy gateways stamp an LMS token before forwarding traffic to the provider zone,
	// so the real route must trust the LMS issuer of every zone that has a proxy route.
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	for _, zoneRef := range allRelevantZones {
		z, err := util.GetZone(ctx, scopedClient, zoneRef.K8s())
		if err != nil {
			return errors.Wrapf(err, "failed to get zone %s for LMS issuer collection", zoneRef.Name)
		}
		if z.Status.Links.LmsIssuer != "" {
			state.crossZoneLmsIssuers = append(state.crossZoneLmsIssuers, z.Status.Links.LmsIssuer)
		}
	}

	// --- Create proxy routes ---
	apiExp.Status.ProxyRoutes = nil

	for _, subscriberZoneRef := range allRelevantZones {
		options := []util.CreateRouteOption{
			util.WithRealmName(state.realmName),
		}

		// Pass provider failover zone if configured (so the proxy route knows the secondary target)
		if apiExp.HasFailover() {
			options = append(options, util.WithFailoverZone(apiExp.Spec.Traffic.Failover.Zones[0]))
		}

		if apiExp.HasProviderRateLimit() {
			options = append(options, util.WithServiceRateLimit(apiExp.Spec.Traffic.RateLimit.Provider))
		}

		// Consumer failover enrichment: add all failover hostnames, paths, and IDP issuers
		if state.hasConsumerFailover {
			options = append(options,
				util.WithAdditionalHostnames(state.consumerFailoverHosts...),
				util.WithAdditionalPaths(state.consumerFailoverPaths...),
				util.AddTrustedIssuers(state.consumerFailoverIssuers...),
			)
		}

		proxyRoute, err := util.CreateProxyRoute(ctx, subscriberZoneRef, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath, options...)
		if err != nil {
			return errors.Wrapf(err, "failed to create proxy route for zone %s", subscriberZoneRef.Name)
		}

		apiExp.Status.ProxyRoutes = append(apiExp.Status.ProxyRoutes, *types.ObjectRefFromObject(proxyRoute))
		logger.V(1).Info("Proxy route created/updated", "zone", subscriberZoneRef.Name, "route", proxyRoute.Name)
	}

	// --- Provider failover (secondary) route ---
	if apiExp.HasFailover() {
		failoverZone := apiExp.Spec.Traffic.Failover.Zones[0] // currently only one failover zone is supported
		failoverRoute, err := util.CreateProxyRoute(ctx, failoverZone, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			util.WithFailoverUpstreams(apiExp.Spec.Upstreams...),
			util.WithFailoverSecurity(apiExp.Spec.Security),
			util.WithTrustedIssuers(state.crossZoneLmsIssuers),
			util.WithRealmName(state.realmName),
		)
		if err != nil {
			return errors.Wrapf(err, "unable to create failover route for apiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
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

	return nil
}

// collectCrossZoneSubscriberZones returns the set of subscriber zones that need
// a proxy route (i.e. not local to the exposure and not already covered by provider failover).
func (h *ApiExposureHandler) collectCrossZoneSubscriberZones(apiExp *apiapi.ApiExposure, state *routingState) []types.ObjectRef {
	var zones []types.ObjectRef

	for _, sub := range state.subscribers {
		if sub.Spec.Zone.Equals(&apiExp.Spec.Zone) {
			continue
		}
		if apiExp.HasFailover() {
			alreadySecondaryRoute := slices.ContainsFunc(apiExp.Spec.Traffic.Failover.Zones, func(failoverZone types.ObjectRef) bool {
				return failoverZone.Equals(&sub.Spec.Zone)
			})
			if alreadySecondaryRoute {
				continue
			}
		}

		if !slices.Contains(zones, sub.Spec.Zone) {
			zones = append(zones, sub.Spec.Zone)
		}
	}

	return zones
}

// collectConsumerFailoverEnrichment populates consumer failover hosts, paths, and issuers
// on state, and extends allRelevantZones with any additional failover-eligible zones that
// need proxy routes.
func (h *ApiExposureHandler) collectConsumerFailoverEnrichment(ctx context.Context, apiExp *apiapi.ApiExposure, state *routingState, allRelevantZones []types.ObjectRef) ([]types.ObjectRef, error) {
	logger := log.FromContext(ctx)

	availableFailoverZones, err := util.FindAllZonesWithFeatureEnabled(ctx, adminv1.FeatureConsumerFailover)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find zones with consumer failover feature enabled for apiExposure: %s", apiExp.Name)
	}

	if len(availableFailoverZones) == 0 {
		logger.Info("No zones with consumer failover feature enabled found, skipping failover configuration")
		return allRelevantZones, nil
	}

	for _, zone := range availableFailoverZones {
		if apiExp.HasFailover() {
			alreadySecondaryRoute := slices.ContainsFunc(apiExp.Spec.Traffic.Failover.Zones, func(failoverZone types.ObjectRef) bool {
				return failoverZone.Equals(zone)
			})
			if alreadySecondaryRoute {
				continue
			}
		}

		preset, err := zone.SelectGatewayPreset(adminv1.FeatureConsumerFailover)
		if err != nil {
			return nil, ctrlerrors.BlockedErrorf("Zone %q does not have a gateway preset with consumer failover feature enabled", zone.Name)
		}

		hosts, paths := preset.ResolveHostnamesAndPaths(apiExp.Spec.ApiBasePath)
		state.consumerFailoverHosts = append(state.consumerFailoverHosts, hosts...)
		state.consumerFailoverPaths = append(state.consumerFailoverPaths, paths...)
		state.consumerFailoverIssuers = append(state.consumerFailoverIssuers, zone.Status.Links.Issuer)

		if apiExp.Spec.Zone.Equals(zone) {
			continue
		}

		objRef := types.ObjectRefFromObject(zone)
		if !slices.Contains(allRelevantZones, *objRef) {
			allRelevantZones = append(allRelevantZones, *objRef)
		}
	}

	// Deduplicate (presets from different zones could theoretically overlap)
	slices.Sort(state.consumerFailoverHosts)
	slices.Sort(state.consumerFailoverPaths)
	state.consumerFailoverHosts = slices.Compact(slices.Clip(state.consumerFailoverHosts))
	state.consumerFailoverPaths = slices.Compact(slices.Clip(state.consumerFailoverPaths))

	return allRelevantZones, nil
}

// createRealRoute creates the real (primary) route for the ApiExposure using the
// pre-computed routingState. The real route's TrustedIssuers include:
//   - The exposure zone's own IDP issuer (if local subscribers exist)
//   - LMS issuers from proxy zones (mesh trust for cross-zone traffic)
//   - IDP issuers from all consumer-failover-eligible zones (direct consumer failover)
func (h *ApiExposureHandler) createRealRoute(ctx context.Context, apiExp *apiapi.ApiExposure, state *routingState) (*gatewayapi.Route, error) {
	options := []util.CreateRouteOption{
		util.WithRealmName(state.realmName),
		util.WithProxyTarget(state.hasCrossZoneSubs),
	}

	// Build TrustedIssuers for the real route
	var trustedIssuers []string
	if state.hasLocalSubs && state.exposureZone.Status.Links.Issuer != "" {
		trustedIssuers = append(trustedIssuers, state.exposureZone.Status.Links.Issuer)
	}
	trustedIssuers = append(trustedIssuers, state.crossZoneLmsIssuers...)
	trustedIssuers = append(trustedIssuers, state.consumerFailoverIssuers...)
	options = append(options, util.WithTrustedIssuers(trustedIssuers))

	// Consumer failover: the real route also gets all failover hostnames/paths so that
	// consumers failing over directly to the provider zone's gateway can reach it.
	if len(state.consumerFailoverHosts) > 0 {
		options = append(options,
			util.WithAdditionalHostnames(state.consumerFailoverHosts...),
			util.WithAdditionalPaths(state.consumerFailoverPaths...),
		)
	}

	return util.CreateRealRoute(ctx, apiExp.Spec.Zone, apiExp, options...)
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
			// Defensive fallback: team-not-found should not happen in normal lifecycle,
			// because team deletion removes the namespace and with it ApiExposures.
			log.FromContext(ctx).V(1).Info("Skipping ApiCategory policy validation because team was not found")
			return true
		}
		msg := util.BuildApiCategoryPolicyResolutionMessage(api.Spec.Category, err)
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
		return false
	}

	apiCategory, err := util.ResolveActiveApiCategoryForApi(ctx, api)
	if err == nil {
		teamCategory := string(team.Spec.Category)
		if !apiCategory.IsAllowedForTeamCategory(teamCategory) {
			msg := util.BuildApiCategoryExposureDeniedMessage(teamCategory, apiCategory.Spec.LabelValue)
			apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryTeamCategoryNotAllowedReason, msg))
			apiExp.SetCondition(condition.NewBlockedCondition(msg))
			return false
		}
		return true
	}

	if apierrors.IsNotFound(err) {
		log.FromContext(ctx).V(1).Info("Skipping ApiCategory policy validation because no ApiCategories exist")
		return true
	}

	msg := util.BuildApiCategoryPolicyResolutionMessage(api.Spec.Category, err)
	blockedErr, isBlocked := stderrors.AsType[ctrlerrors.BlockedError](err)
	retryableErr, isRetryable := stderrors.AsType[ctrlerrors.RetryableError](err)
	switch {
	case isBlocked:
		log.FromContext(ctx).V(1).Info("ApiCategory policy validation blocked", "reason", blockedErr.Error())
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
	case isRetryable:
		log.FromContext(ctx).V(1).Info("ApiCategory policy validation retryable", "reason", retryableErr.Error())
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
	default:
		log.FromContext(ctx).V(1).Info("ApiCategory policy validation failed", "reason", err.Error())
		apiExp.SetCondition(condition.NewNotReadyCondition(util.ApiCategoryPolicyResolutionFailedReason, msg))
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
	}
	return false
}
