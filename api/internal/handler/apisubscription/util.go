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
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ApiMustExist checks if the ApiSubscription has a corresponding active Api.
// If not, it sets appropriate conditions on the ApiSubscription and cleans up child resources.
func ApiMustExist(ctx context.Context, apiSub *apiapi.ApiSubscription) (*apiapi.Api, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	found, api, err := util.FindActiveAPI(ctx, apiSub.Spec.ApiBasePath)
	if err != nil {
		return nil, err
	}
	if !found {
		apiSub.SetCondition(condition.NewNotReadyCondition("NoApi",
			fmt.Sprintf("API %q is not registered. Cannot provision ApiSubscription", apiSub.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not registered. ApiExposure will be automatically processed, when the API is registered and exposed", apiSub.Spec.ApiBasePath)
		apiSub.SetCondition(condition.NewBlockedCondition(msg))

		deleted, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", apiSub.GetName(), apiSub.GetNamespace())
		}
		log.Info("🧹 No active API found. Cleaning up Consumer of ApiSubscription", "basePath", apiSub.Spec.ApiBasePath, "deleted", deleted)
	}

	return api, nil
}

func ApiExposureMustExist(ctx context.Context, apiSub *apiapi.ApiSubscription) (*apiapi.ApiExposure, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	found, apiExp, err := util.FindActiveAPIExposure(ctx, apiSub.Spec.ApiBasePath)
	if err != nil {
		return nil, err
	}
	if !found {
		log.Info("no active ApiExposure found", "basePath", apiSub.Spec.ApiBasePath)
		apiSub.SetCondition(condition.NewNotReadyCondition("NoApiExposure",
			fmt.Sprintf("API %q is not exposed. Cannot provision ApiSubscription", apiSub.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not exposed. ApiSubscription will be automatically processed, when the API is exposed", apiSub.Spec.ApiBasePath)
		apiSub.SetCondition(condition.NewBlockedCondition(msg))

		// clean up consumeRoute subresource
		deleted, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return nil, errors.Wrapf(err,
				"Unable to cleanup consumeroutes for Apisubscription:  %s in namespace: %s", apiSub.GetName(), apiSub.GetNamespace())
		}
		log.Info("🧹 No active API-Exposure found. Cleaning up Consumer of ApiSubscription", "basePath", apiSub.Spec.ApiBasePath, "deleted", deleted)
	}

	return apiExp, nil
}

// ApiVisibilityMustBeValid checks if the ApiSubscription is valid for the given ApiExposure visibility
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
