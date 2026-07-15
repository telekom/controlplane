// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

const stringTrue = "true"

var ApplicationLabelKey = config.BuildLabelKey("application")

const (
	ApiCategoryPolicyResolutionFailedReason = "ApiCategoryPolicyResolutionFailed"
	ApiCategoryTeamCategoryNotAllowedReason = "ApiCategoryTeamCategoryNotAllowed"
)

// GetZone retrieves a Zone object based on the provided ObjectRef for a zone.
func GetZone(ctx context.Context, scopedClient cclient.ScopedClient, ref client.ObjectKey) (*adminapi.Zone, error) {
	zone := &adminapi.Zone{}
	err := scopedClient.Get(ctx, ref, zone)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to find zone %q", ref.String()))
		}
		return nil, ctrlerrors.BlockedErrorf("zone %q not found", ref.String())
	}

	return zone, nil
}

func GetApplicationFromLabel(ctx context.Context, apiExposure *apiv1.ApiExposure) (*applicationapi.Application, error) {
	applicationLabel, ok := apiExposure.GetObjectMeta().GetLabels()[ApplicationLabelKey]
	if !ok {
		return nil, errors.New("Application label not found in API Exposure")
	}

	ref := types.ObjectRef{
		Name:      applicationLabel,
		Namespace: apiExposure.Namespace,
	}

	return GetApplication(ctx, ref)
}

func GetApplication(ctx context.Context, ref types.ObjectRef) (*applicationapi.Application, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	application := &applicationapi.Application{}
	err := scopedClient.Get(ctx, ref.K8s(), application)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to find application %q", ref.String()))
		}
		return nil, ctrlerrors.BlockedErrorf("application %q not found", ref.String())
	}
	if err := condition.EnsureReady(application); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.String())
	}

	return application, nil
}

// GetDefaultPresetForZone fetches the Zone for the given ref and returns its default gateway preset.
func GetDefaultPresetForZone(ctx context.Context, zoneRef types.ObjectRef) (*adminapi.GatewayConfigPreset, *adminapi.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone, err := GetZone(ctx, c, zoneRef.K8s())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get zone %s", zoneRef.String())
	}

	preset, err := zone.Spec.Gateway.GetDefaultPreset()
	if err != nil {
		return nil, zone, errors.Wrapf(err, "failed to get default preset for zone %s", zoneRef.String())
	}

	return preset, zone, nil
}

// GetPresetForZone fetches the Zone for the given ref and returns the gateway preset with the given name.
func GetPresetForZone(ctx context.Context, zoneRef types.ObjectRef, presetName string) (*adminapi.GatewayConfigPreset, *adminapi.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone, err := GetZone(ctx, c, zoneRef.K8s())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get zone %s", zoneRef.String())
	}

	preset, err := zone.Spec.Gateway.GetPresetByName(presetName)
	if err != nil {
		return nil, zone, errors.Wrapf(err, "failed to get preset %q for zone %s", presetName, zoneRef.String())
	}

	return preset, zone, nil
}

// FindAPI checks if there is an active Api corresponding to the given apiBasePath.
func FindActiveAPI(ctx context.Context, apiBasePath string) (bool, *apiv1.Api, error) {
	logger := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiBasePathLabelValue := labelutil.NormalizeLabelValue(apiBasePath)

	apiList := &apiv1.ApiList{}
	err := scopedClient.List(ctx, apiList,
		client.MatchingLabels{apiv1.BasePathLabelKey: apiBasePathLabelValue},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding Apis for ApiBasePath: %q", apiBasePathLabelValue)
	}

	var relevantApi *apiv1.Api

	switch len(apiList.Items) {
	case 0:
		return false, nil, nil
	case 1:
		relevantApi = &apiList.Items[0]
	default:
		// This should never happens as the Api-Handler ensures uniqueness of active Apis per BasePath
		// sort the list by creation timestamp and get the oldest one
		sort.Slice(apiList.Items, func(i, j int) bool {
			return apiList.Items[i].CreationTimestamp.Before(&apiList.Items[j].CreationTimestamp)
		})
		relevantApi = &apiList.Items[0]
		logger.Info("⚠️  Multiple active Apis found for the same BasePath. Using the oldest one.", "basePath", apiBasePathLabelValue, "apiName", relevantApi.Name)
	}

	if err := condition.EnsureReady(relevantApi); err != nil {
		return false, relevantApi, ctrlerrors.BlockedErrorf("No API %q is ready.", apiBasePath)
	}

	return true, relevantApi, nil
}

func ResolveActiveApiCategoryForApi(ctx context.Context, api *apiv1.Api) (*apiv1.ApiCategory, error) {
	if api == nil {
		return nil, errors.New("api is nil")
	}
	return ResolveActiveApiCategoryByLabelValue(ctx, api.Spec.Category)
}

func ResolveActiveApiCategoryByLabelValue(ctx context.Context, labelValue string) (*apiv1.ApiCategory, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	normalizedLabelValue := strings.TrimSpace(labelValue)
	if normalizedLabelValue == "" {
		return nil, ctrlerrors.BlockedErrorf("ApiCategory label value is empty")
	}

	apiCategoryList := &apiv1.ApiCategoryList{}
	if err := scopedClient.List(ctx, apiCategoryList, client.MatchingFields{"spec.labelValue": normalizedLabelValue}); err != nil {
		return nil, errors.Wrap(err, "failed to list ApiCategories by label value")
	}
	if len(apiCategoryList.Items) == 0 {
		anyApiCategoryList := &apiv1.ApiCategoryList{}
		if err := scopedClient.List(ctx, anyApiCategoryList, client.Limit(1)); err != nil {
			return nil, errors.Wrap(err, "failed to verify whether ApiCategories exist")
		}
		if len(anyApiCategoryList.Items) == 0 {
			return nil, apierrors.NewNotFound(schema.GroupResource{Group: apiv1.GroupVersion.Group, Resource: "apicategories"}, normalizedLabelValue)
		}
		return nil, ctrlerrors.BlockedErrorf("ApiCategory %q not found", normalizedLabelValue)
	}
	apiCategory := &apiCategoryList.Items[0]
	if !apiCategory.Spec.Active {
		return nil, ctrlerrors.BlockedErrorf("ApiCategory %q is not active", normalizedLabelValue)
	}
	return apiCategory, nil
}

func BuildApiCategoryPolicyResolutionMessage(apiCategoryLabel string, err error) string {
	return fmt.Sprintf("ApiCategory policy for category %q cannot be resolved: %v", apiCategoryLabel, err)
}

func BuildApiCategoryExposureDeniedMessage(teamCategory, apiCategoryLabel string) string {
	return fmt.Sprintf("Team with category %q is not allowed to expose APIs of category %q", teamCategory, apiCategoryLabel)
}

func BuildApiCategorySubscriptionDeniedMessage(teamCategory, apiCategoryLabel string) string {
	return fmt.Sprintf("Team with category %q is not allowed to subscribe to APIs of category %q", teamCategory, apiCategoryLabel)
}

// FindAPIExposure checks if there is an active ApiExposure corresponding to the given apiBasePath.
func FindActiveAPIExposure(ctx context.Context, apiBasePath string) (bool, *apiv1.ApiExposure, error) {
	logger := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiBasePathLabelValue := labelutil.NormalizeLabelValue(apiBasePath)

	apiExposureList := &apiv1.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiv1.BasePathLabelKey: apiBasePathLabelValue},
		client.MatchingFields{"status.active": stringTrue})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding ApiExposures for ApiBasePath: %q", apiBasePathLabelValue)
	}

	logger.V(1).Info("found active ApiExposures", "size", len(apiExposureList.Items), "basePath", apiBasePathLabelValue)

	var relevantApiExposure *apiv1.ApiExposure

	switch len(apiExposureList.Items) {
	case 0:
		return false, nil, nil
	case 1:
		relevantApiExposure = &apiExposureList.Items[0]
	default:
		// This should never happens as the ApiExposure-Handler ensures uniqueness of active ApiExposures per BasePath
		// sort the list by creation timestamp and get the oldest one
		sort.Slice(apiExposureList.Items, func(i, j int) bool {
			return apiExposureList.Items[i].CreationTimestamp.Before(&apiExposureList.Items[j].CreationTimestamp)
		})
		relevantApiExposure = &apiExposureList.Items[0]
		logger.Info("⚠️  Multiple active ApiExposures found for the same BasePath. Using the oldest one.", "basePath", apiBasePathLabelValue, "apiExposureName", relevantApiExposure.Name)
	}

	if err := condition.EnsureReady(relevantApiExposure); err != nil {
		return false, relevantApiExposure, ctrlerrors.BlockedErrorf("No ApiExposure %q is ready.", apiBasePath)
	}

	return true, relevantApiExposure, nil
}

// FindFailoverEligibleZones lists all zones and returns those that are eligible for failover for a given zone.
func FindFailoverEligibleZones(ctx context.Context, myZone types.ObjectRef) ([]types.ObjectRef, error) {
	allZones, err := FindAllZonesWithFeatureEnabled(ctx, adminapi.FeatureConsumerFailover)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find zones with consumer failover feature enabled")
	}

	var eligibleZones []types.ObjectRef
	for _, zone := range allZones {
		// Skip same-zone as myZone (a zone cannot failover to itself)
		if zone.Name == myZone.Name {
			continue
		}
		eligibleZones = append(eligibleZones, *types.ObjectRefFromObject(zone))
	}

	return eligibleZones, nil
}

// FindAllSubscribersForApiExposure lists all ApiSubscriptions for a given ApiExposure that are approved and have a matching basepath.
func FindAllSubscribersForApiExposure(ctx context.Context, apiExp *apiv1.ApiExposure) ([]*apiv1.ApiSubscription, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	subList := &apiv1.ApiSubscriptionList{}
	if err := c.List(ctx, subList, client.MatchingLabels{
		apiv1.BasePathLabelKey: labelutil.NormalizeLabelValue(apiExp.Spec.ApiBasePath),
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to list ApiSubscriptions for basepath %q", apiExp.Spec.ApiBasePath)
	}

	var subscribers []*apiv1.ApiSubscription

	for i := range subList.Items {
		sub := &subList.Items[i]

		// Skip subscriptions that are being deleted; their finalizer may still
		// be running, but they should no longer influence proxy route creation.
		if controller.IsBeingDeleted(sub) {
			continue
		}

		// Skip if basepath doesn't match exactly (label normalization can cause collisions)
		if sub.Spec.ApiBasePath != apiExp.Spec.ApiBasePath {
			logger.V(1).Info("Skipping subscription with non-matching basepath",
				"subscription", sub.Name, "subscriptionBasePath", sub.Spec.ApiBasePath, "exposureBasePath", apiExp.Spec.ApiBasePath)
			continue
		}

		// We could check the Approval state here, however, we MUST NEVER remove a Route when a subscription (which was approved previously) is updated.
		// A pending or rejected ApprovalRequest MUST NOT remove a Route, as this would break the API for the consumer.
		// Therefore, we do not filter by Approval state here.

		subscribers = append(subscribers, sub)
	}

	return subscribers, nil
}

// FindAllZonesWithFeatureEnabled lists all zones and returns those that have the given feature enabled.
func FindAllZonesWithFeatureEnabled(ctx context.Context, featureName adminapi.FeatureName) ([]*adminapi.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zoneList := &adminapi.ZoneList{}
	if err := c.List(ctx, zoneList); err != nil {
		return nil, errors.Wrap(err, "failed to list zones")
	}

	var zonesWithFeature []*adminapi.Zone
	for i := range zoneList.Items {
		zone := &zoneList.Items[i]

		// Skip zones that are being deleted; their finalizer may still
		// be running, but they should no longer be considered for feature checks.
		if controller.IsBeingDeleted(zone) {
			continue
		}

		if zone.IsFeatureEnabled(featureName) {
			zonesWithFeature = append(zonesWithFeature, zone)
		}
	}

	return zonesWithFeature, nil
}
