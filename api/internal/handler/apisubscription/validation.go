// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"slices"

	"github.com/pkg/errors"
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

// Scopes must exist in the Api specification
func ScopesMustExist(ctx context.Context, apiSub *apiapi.ApiSubscription) (bool, error) {
	log := log.FromContext(ctx)
	scopedClient := cclient.ClientFromContextOrDie(ctx)

	api := &apiapi.Api{}
	err := scopedClient.Get(ctx, client.ObjectKey{
		Name:      apiSub.GetLabels()[apiapi.BasePathLabelKey],
		Namespace: apiSub.GetNamespace(),
	}, api)
	if err != nil {
		return false, errors.Wrapf(err,
			"failed to get Api for ApiSubscription: %s in namespace: %s", apiSub.GetName(), apiSub.GetNamespace())
	}

	if len(api.Spec.Security.Authentication.OAuth2.Scopes) == 0 {
		log.Info("❌ No scopes defined in Api specification. ApiSubscription is blocked")
		apiSub.SetCondition(condition.NewNotReadyCondition("NoScopesDefined", "No scopes defined in Api specification"))
		apiSub.SetCondition(condition.NewBlockedCondition(
			"No scopes defined in Api specification. ApiSubscription will be automatically processed, if the API will be updated with scopes"))
		return false, nil
	}

	// Check if scopes are a subset of the Api specification
	for _, scope := range apiSub.Spec.Security.Authentication.OAuth2.Scopes {
		if !slices.Contains(api.Spec.Security.Authentication.OAuth2.Scopes, scope) {
			log.Info("❌ Scope is not defined in Api specification. ApiSubscription is blocked", "scope", scope)

			// If scope is not defined in the Api specification, set conditions and return
			apiSub.SetCondition(condition.NewNotReadyCondition("ScopeNotDefined", "Scope is not defined in Api specification"))
			apiSub.SetCondition(condition.NewBlockedCondition(
				"Scope is not defined in Api specification. ApiSubscription will be automatically processed, if the API will be updated with scopes"))

			return false, nil
		}
	}

	return true, nil
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
