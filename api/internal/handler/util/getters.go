// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"github.com/telekom/controlplane/common/pkg/config"
	"sort"

	"github.com/pkg/errors"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ApplicationLabelKey = config.BuildLabelKey("application")
)

// GetZone retrieves a Zone object based on the provided ObjectRef for a zone.
func GetZone(ctx context.Context, client cclient.ScopedClient, ref client.ObjectKey) (*adminapi.Zone, error) {
	zone := &adminapi.Zone{}
	err := client.Get(ctx, ref, zone)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to find zone %q", ref.String()))
		}
		return nil, ctrlerrors.BlockedErrorf("zone %q not found", ref.String())
	}
	if err := condition.EnsureReady(zone); err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q is not ready", ref.String())
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
	client := cclient.ClientFromContextOrDie(ctx)

	application := &applicationapi.Application{}
	err := client.Get(ctx, ref.K8s(), application)
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

func GetRealm(ctx context.Context, ref client.ObjectKey) (*gatewayapi.Realm, error) {
	client := cclient.ClientFromContextOrDie(ctx)

	realm := &gatewayapi.Realm{}
	err := client.Get(ctx, ref, realm)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to find realm %q", ref.String()))
		}
		return nil, ctrlerrors.BlockedErrorf("realm %q not found", ref.String())
	}
	if err := condition.EnsureReady(realm); err != nil {
		return nil, ctrlerrors.BlockedErrorf("realm %q is not ready", ref.String())
	}

	return realm, nil
}

func GetRealmForZone(ctx context.Context, zoneRef types.ObjectRef, realmName string) (*gatewayapi.Realm, *adminapi.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone, err := GetZone(ctx, c, zoneRef.K8s())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get zone %s", zoneRef.String())
	}

	realmRef := client.ObjectKey{
		Name:      realmName,
		Namespace: zone.Status.Namespace,
	}
	realm, err := GetRealm(ctx, realmRef)
	if err != nil {
		return nil, zone, errors.Wrapf(err, "failed to get realm %s", realmRef.String())
	}

	return realm, zone, nil
}

// FindAPI checks if there is an active Api corresponding to the given apiBasePath.
func FindActiveAPI(ctx context.Context, apiBasePath string) (bool, *apiv1.Api, error) {
	log := log.FromContext(ctx)
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
		log.Info("⚠️  Multiple active Apis found for the same BasePath. Using the oldest one.", "basePath", apiBasePathLabelValue, "apiName", relevantApi.Name)
	}

	if err := condition.EnsureReady(relevantApi); err != nil {
		return false, relevantApi, ctrlerrors.BlockedErrorf("No API %q is ready.", apiBasePath)
	}

	return true, relevantApi, nil
}

// FindAPIExposure checks if there is an active ApiExposure corresponding to the given apiBasePath.
func FindActiveAPIExposure(ctx context.Context, apiBasePath string) (bool, *apiv1.ApiExposure, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiBasePathLabelValue := labelutil.NormalizeLabelValue(apiBasePath)

	apiExposureList := &apiv1.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiv1.BasePathLabelKey: apiBasePathLabelValue},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding ApiExposures for ApiBasePath: %q", apiBasePathLabelValue)
	}

	log.V(1).Info("found active ApiExposures", "size", len(apiExposureList.Items), "basePath", apiBasePathLabelValue)

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
		log.Info("⚠️  Multiple active ApiExposures found for the same BasePath. Using the oldest one.", "basePath", apiBasePathLabelValue, "apiExposureName", relevantApiExposure.Name)
	}

	if err := condition.EnsureReady(relevantApiExposure); err != nil {
		return false, relevantApiExposure, ctrlerrors.BlockedErrorf("No ApiExposure %q is ready.", apiBasePath)
	}

	return true, relevantApiExposure, nil
}
