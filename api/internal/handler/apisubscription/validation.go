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
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func ApiMustExist(ctx context.Context, apiSub *apiapi.ApiSubscription) (bool, *apiapi.Api, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiList := &apiapi.ApiList{}
	err := scopedClient.List(ctx, apiList,
		client.MatchingLabels{apiapi.BasePathLabelKey: apiSub.GetLabels()[apiapi.BasePathLabelKey]},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding APIs for ApiSubscription: %s in namespace: %s ", apiSub.GetName(), apiSub.GetNamespace())
	}

	// if no corresponding active api is found, set conditions and return
	if len(apiList.Items) == 0 {
		apiSub.SetCondition(condition.NewNotReadyCondition("NoApi",
			fmt.Sprintf("API %q is not registered. Cannot provision ApiSubscription", apiSub.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not registered. ApiSubscription will be automatically processed, when the API is registered", apiSub.Spec.ApiBasePath)
		apiSub.SetCondition(condition.NewBlockedCondition(msg))
		log.Info("❌ API is not yet registered. ApiSubscription is blocked")

		// clean up consumeRoute subresource
		log.Info("🧹 In this case we would delete the child resources")
		_, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return false, nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", apiSub.GetName(), apiSub.GetNamespace())
		}

		return false, nil, nil
	}

	return true, &apiList.Items[0], nil
}

func ApiExposureMustExist(ctx context.Context, apiSub *apiapi.ApiSubscription) (bool, *apiapi.ApiExposure, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	apiExposureList := &apiapi.ApiExposureList{}
	err := scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiapi.BasePathLabelKey: apiSub.GetLabels()[apiapi.BasePathLabelKey]},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return false, nil, errors.Wrapf(err,
			"failed to list corresponding ApiExposures for ApiSubscription: %s in namespace: %s",
			apiSub.GetName(), apiSub.GetNamespace())
	}

	// if no corresponding active apiExposure is found, set conditions and return
	if len(apiExposureList.Items) == 0 {
		apiSub.SetCondition(condition.NewNotReadyCondition("NoApiExposure",
			fmt.Sprintf("API %q is not exposed. Cannot provision ApiSubscription", apiSub.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not exposed. ApiSubscription will be automatically processed, when the API is exposed", apiSub.Spec.ApiBasePath)
		apiSub.SetCondition(condition.NewBlockedCondition(msg))

		log.Info("❌ API is not yet exposed. ApiSubscription is blocked")

		// clean up consumeRoute subresource
		log.Info("🧹 In this case we would delete the child resources")
		_, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return false, nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", apiSub.GetName(), apiSub.GetNamespace())
		}

		return false, nil, nil
	}

	return true, &apiExposureList.Items[0], nil
}

func ApiVisibilityMustBeValid(ctx context.Context, apiExposure *apiapi.ApiExposure, apiSubscription *apiapi.ApiSubscription) (bool, error) {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	log := log.FromContext(ctx)

	exposureVisibility := apiExposure.Spec.Visibility

	// any subscription is valid for a WORLD exposure
	if exposureVisibility == apiapi.VisibilityWorld {
		return true, nil
	}

	// get the subscription zone
	subZone := &adminv1.Zone{}
	err := scopedClient.Get(ctx, apiSubscription.Spec.Zone.K8s(), subZone)
	if err != nil {
		log.Error(err, "unable to get zone", "name", apiSubscription.Spec.Zone.K8s())
		return false, errors.Wrapf(err, "Zone '%s' not found", apiSubscription.Spec.Zone.GetName())
	}

	// only same zone
	if exposureVisibility == apiapi.VisibilityZone {
		if apiExposure.Spec.Zone.GetName() != subZone.GetName() {
			log.Info(fmt.Sprintf("Exposure visibility is ZONE and it doesnt match the subscription zone '%s'", subZone.GetName()))
			return false, nil
		}
	}

	// only enterprise zones
	if exposureVisibility == apiapi.VisibilityEnterprise {
		if subZone.Spec.Visibility != adminv1.ZoneVisibilityEnterprise {
			log.Info(fmt.Sprintf("Api is exposed with visibility '%s', but subscriptions is from zone with visibility '%s'", apiapi.VisibilityEnterprise, subZone.Spec.Visibility))
			return false, nil
		}
	}
	return true, nil
}
