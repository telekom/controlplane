// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func ApiMustExist(ctx context.Context, obj types.Object) (bool, *apiapi.Api, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiList := &apiapi.ApiList{}
	err := scopedClient.List(ctx, apiList,
		client.MatchingLabels{apiapi.BasePathLabelKey: obj.GetLabels()[apiapi.BasePathLabelKey]},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding APIs for ApiSubscription: %s in namespace: %s ", obj.GetName(), obj.GetNamespace())
	}

	// if no corresponding active api is found, set conditions and return
	if len(apiList.Items) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("NoApiRegistered", "API is not yet registered"))
		obj.SetCondition(condition.NewBlockedCondition(
			"API is not yet registered. ApiSubscription will be automatically processed, if the API will be registered"))
		log.Info("❌ API is not yet registered. ApiSubscription is blocked")

		// clean up consumeRoute subresource
		log.Info("🧹 In this case we would delete the child resources")
		_, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(obj))
		if err != nil {
			return false, nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", obj.GetName(), obj.GetNamespace())
		}

		return false, nil, nil
	}

	return true, &apiList.Items[0], nil
}

func ApiExposureMustExist(ctx context.Context, obj types.Object) (bool, *apiapi.ApiExposure, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiExposureList := &apiapi.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiapi.BasePathLabelKey: obj.GetLabels()[apiapi.BasePathLabelKey]},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding ApiExposures for ApiSubscription: %s in namespace: %s",
			obj.GetName(), obj.GetNamespace())
	}

	// if no corresponding active apiExposure is found, set conditions and return
	if len(apiExposureList.Items) == 0 {
		obj.SetCondition(condition.NewNotReadyCondition("NoApiExposure", "API is not yet exposed"))
		obj.SetCondition(condition.NewBlockedCondition(
			"API is not yet exposed. ApiSubscription will be automatically processed, if the API will be exposed"))
		log.Info("❌ API is not yet exposed. ApiSubscription is blocked")

		// clean up consumeRoute subresource
		log.Info("🧹 In this case we would delete the child resources")
		_, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(obj))
		if err != nil {
			return false, nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", obj.GetName(), obj.GetNamespace())
		}

		return false, nil, nil
	}

	return true, &apiExposureList.Items[0], nil
}

func ApiVisibilityMustBeValid(ctx context.Context, apiExposure *apiapi.ApiExposure, apiSubscription *apiapi.ApiSubscription) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	exposureVisibility := apiExposure.Spec.Visibility

	// any subscription is valid for a WORLD exposure
	if exposureVisibility == apiapi.VisibilityWorld {
		return nil
	}

	// get the subscription zone
	subZone := &adminv1.Zone{}
	err := scopedClient.Get(ctx, apiSubscription.Spec.Zone.K8s(), subZone)
	if err != nil {
		log.Error(err, "unable to get zone", "name", apiSubscription.Spec.Zone.K8s())
		return errors.Wrapf(err, "Zone '%s' not found", apiSubscription.Spec.Zone.GetName())
	}

	// only same zone
	if exposureVisibility == apiapi.VisibilityZone {
		if apiExposure.Spec.Zone.GetName() != subZone.GetName() {
			return errors.New("Exposure visibility is ZONE and it doesnt match the subscription zone '" + subZone.GetName() + "'")
		}
	}

	// only enterprise zones
	if exposureVisibility == apiapi.VisibilityEnterprise {
		if subZone.Spec.Visibility != adminv1.ZoneVisibilityEnterprise {
			return errors.New(fmt.Sprintf("Api is exposed with visibility '%s', but subscriptions is from zone with visibility '%s'", apiapi.VisibilityEnterprise, subZone.Spec.Visibility))
		}
	}
	return nil
}
