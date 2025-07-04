// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*apiapi.ApiExposure] = (*ApiExposureHandler)(nil)

type ApiExposureHandler struct{}

func (h *ApiExposureHandler) CreateOrUpdate(ctx context.Context, obj *apiapi.ApiExposure) error {
	log := log.FromContext(ctx)

	scopedClient := cclient.ClientFromContextOrDie(ctx)

	//  get corresponding active api
	apiList := &apiapi.ApiList{}
	err := scopedClient.List(ctx, apiList,
		client.MatchingLabels{apiapi.BasePathLabelKey: labelutil.NormalizeValue(obj.Spec.ApiBasePath)},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return errors.Wrapf(err,
			"failed to list corresponding APIs for ApiExposure: %s in namespace: %s", obj.Name, obj.Namespace)
	}

	// if no corresponding active api is found, set conditions and return
	if len(apiList.Items) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("NoApiRegistered", "API is not yet registered"))
		obj.SetCondition(condition.NewBlockedCondition(
			"API is not yet registered. ApiExposure will be automatically processed, if the API will be registered"))
		log.Info("❌ API is not yet registered. ApiExposure is blocked")

		routeList := &gatewayapi.RouteList{}
		// Using ownedByLabel to cleanup all routes that are owned by the ApiExposure
		_, err := scopedClient.Cleanup(ctx, routeList, cclient.OwnedByLabel(obj))
		if err != nil {
			return errors.Wrapf(err,
				"failed to cleanup owned routes for ApiExposure: %s in namespace: %s", obj.Name, obj.Namespace)
		}
		return nil
	}
	api := apiList.Items[0]

	// validate if basepathes of the api and apiexposure are really equal
	if api.Spec.BasePath != obj.Spec.ApiBasePath {
		return errors.Wrapf(err,
			"Exposures basePath: %s does not match the APIs basepath: %s", obj.Spec.ApiBasePath, api.Spec.BasePath)
	}

	// check if there is already a different active apiExposure with same basepath
	apiExposureList := &apiapi.ApiExposureList{}
	err = scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiapi.BasePathLabelKey: obj.Labels[apiapi.BasePathLabelKey]})
	if err != nil {
		return errors.Wrap(err, "failed to list ApiExposures")
	}

	// sort the list by creation timestamp and get the oldest one
	sort.Slice(apiExposureList.Items, func(i, j int) bool {
		return apiExposureList.Items[i].CreationTimestamp.Before(&apiExposureList.Items[j].CreationTimestamp)
	})
	apiExposure := apiExposureList.Items[0]

	if apiExposure.Name == obj.Name && apiExposure.Namespace == obj.Namespace {
		// the oldest apiExposure is the same as the one we are trying to handle
		obj.Status.Active = true
	} else {
		// there is already a different apiExposure active with the same BasePathLabelKey
		// the new one will be blocked until the other is deleted
		obj.Status.Active = false
		obj.SetCondition(condition.NewNotReadyCondition("ApiExposureNotActive", "ApiExposure is not active"))
		obj.SetCondition(condition.
			NewBlockedCondition("ApiExposure is blocked, another ApiExposure with the same BasePath is active."))
		log.Info("❌ ApiExposure is blocked, another ApiExposure with the same BasePath is already active.")

		return nil
	}

	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows exposure of api category

	obj.SetCondition(condition.NewProcessingCondition("Provisioning", "Provisioning route"))
	// create real route
	route, err := util.CreateRealRoute(ctx, obj.Spec.Zone, apiExposure.Spec.Upstreams[0].Url, obj.Spec.ApiBasePath, contextutil.EnvFromContextOrDie(ctx))
	if err != nil {
		return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", obj.Name, obj.Namespace)
	}

	if obj.HasFailover() {
		failoverZone := obj.Spec.Traffic.Failover.Zones[0] // currently only one failover zone is supported
		route, err := util.CreateProxyRoute(ctx, failoverZone, obj.Spec.Zone, obj.Spec.ApiBasePath,
			contextutil.EnvFromContextOrDie(ctx), util.WithFailoverUpstreams(apiExposure.Spec.Upstreams...),
		)
		if err != nil {
			return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", obj.Name, obj.Namespace)
		}
		obj.Status.FailoverRoute = types.ObjectRefFromObject(route)
	}

	obj.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	obj.Status.Route = types.ObjectRefFromObject(route)
	log.Info("✅ ApiExposure is processed")

	return nil
}

func (h *ApiExposureHandler) Delete(ctx context.Context, obj *apiapi.ApiExposure) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	if obj.Status.Route != nil {

		route := &gatewayapi.Route{}
		err := scopedClient.Get(ctx, obj.Status.Route.K8s(), route)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrap(err, "failed to get route")
		}

		log.Info("🧹 Deleting real route of exposure")
		err = scopedClient.Delete(ctx, route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}
		log.Info("✅ Successfully deleted obsolete route")
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
		log.Info("🧹 Deleting failover proxy route of exposure")
		err = scopedClient.Delete(ctx, failoverRoute)
		if err != nil {
			return errors.Wrapf(err, "failed to delete failover route")
		}
		log.Info("✅ Successfully deleted obsolete failover route")
	}

	return nil
}
