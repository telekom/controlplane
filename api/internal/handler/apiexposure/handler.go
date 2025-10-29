// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

func (h *ApiExposureHandler) CreateOrUpdate(ctx context.Context, apiExp *apiapi.ApiExposure) error {
	log := log.FromContext(ctx)

	scopedClient := cclient.ClientFromContextOrDie(ctx)

	//  get corresponding active api
	apiList := &apiapi.ApiList{}
	err := scopedClient.List(ctx, apiList,
		client.MatchingLabels{apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(apiExp.Spec.ApiBasePath)},
		client.MatchingFields{"status.active": "true"})
	if err != nil {
		return errors.Wrapf(err,
			"failed to list corresponding APIs for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	// if no corresponding active api is found, set conditions and return
	if len(apiList.Items) == 0 {
		apiExp.SetCondition(condition.NewNotReadyCondition("NoApi",
			fmt.Sprintf("API %q is not registered. Cannot provision ApiExposure", apiExp.Spec.ApiBasePath)),
		)
		msg := fmt.Sprintf("API %q is not registered. ApiExposure will be automatically processed, when the API is registered", apiExp.Spec.ApiBasePath)
		apiExp.SetCondition(condition.NewBlockedCondition(msg))
		log.Info("❌ API is not yet registered. ApiExposure is blocked")

		routeList := &gatewayapi.RouteList{}
		// Using ownedByLabel to cleanup all routes that are owned by the ApiExposure
		_, err := scopedClient.Cleanup(ctx, routeList, cclient.OwnedByLabel(apiExp))
		if err != nil {
			return errors.Wrapf(err,
				"failed to cleanup owned routes for ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
		}
		return nil
	}
	api := apiList.Items[0]

	// validate if basepathes of the api and apiexposure are really equal
	if api.Spec.BasePath != apiExp.Spec.ApiBasePath {
		return errors.Wrapf(err,
			"Exposures basePath: %s does not match the APIs basepath: %s", apiExp.Spec.ApiBasePath, api.Spec.BasePath)
	}

	// check if there is already a different active apiExposure with same basepath
	apiExposureList := &apiapi.ApiExposureList{}
	err = scopedClient.List(ctx, apiExposureList,
		client.MatchingLabels{apiapi.BasePathLabelKey: apiExp.Labels[apiapi.BasePathLabelKey]})
	if err != nil {
		return errors.Wrap(err, "failed to list ApiExposures")
	}

	// sort the list by creation timestamp and get the oldest one
	sort.Slice(apiExposureList.Items, func(i, j int) bool {
		return apiExposureList.Items[i].CreationTimestamp.Before(&apiExposureList.Items[j].CreationTimestamp)
	})
	apiExposure := apiExposureList.Items[0]

	if apiExposure.Name == apiExp.Name && apiExposure.Namespace == apiExp.Namespace {
		// the oldest apiExposure is the same as the one we are trying to handle
		apiExp.Status.Active = true
	} else {
		// there is already a different apiExposure active with the same BasePathLabelKey
		// the new one will be blocked until the other is deleted
		apiExp.Status.Active = false
		apiExp.SetCondition(condition.NewNotReadyCondition("ApiExposureNotActive", "ApiExposure is not active"))
		apiExp.SetCondition(condition.
			NewBlockedCondition("ApiExposure is blocked, another ApiExposure with the same BasePath is active."))
		log.Info("❌ ApiExposure is blocked, another ApiExposure with the same BasePath is already active.")

		return nil
	}

	// Scopes
	// check if scopes exist and scopes are subset from api
	if apiExp.HasM2M() {
		// If scopes are set and its not externalIDP (here its allowed to have unknown/external scopes)
		if apiExp.Spec.Security.M2M.Scopes != nil && !apiExp.HasExternalIdp() {
			if len(api.Spec.Oauth2Scopes) == 0 {
				apiExp.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "Api does not define any Oauth2 scopes"))
				apiExp.SetCondition(condition.NewBlockedCondition("Api does not define any Oauth2 scopes. ApiExposure will be automatically processed, if the API will be updated with scopes"))
				return nil
			} else {
				scopesExist, invalidScopes := util.IsSubsetOfScopes(api.Spec.Oauth2Scopes, apiExp.Spec.Security.M2M.Scopes)
				if !scopesExist {
					var message = fmt.Sprintf("Some defined scopes are not available. Available scopes: \"%s\". Unsupported scopes: \"%s\"",
						strings.Join(api.Spec.Oauth2Scopes, ", "),
						strings.Join(invalidScopes, ", "),
					)
					apiExp.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes which are defined in ApiExposure are not defined in the ApiSpecification"))
					apiExp.SetCondition(condition.NewBlockedCondition(message))
					return nil
				}
			}

			log.V(1).Info("✅ Scopes are valid and exist")
		}
	}

	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows exposure of api category
	// create real route
	route, err := util.CreateRealRoute(ctx, apiExp.Spec.Zone, apiExp, contextutil.EnvFromContextOrDie(ctx))
	if err != nil {
		return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	if apiExp.HasFailover() {
		failoverZone := apiExp.Spec.Traffic.Failover.Zones[0] // currently only one failover zone is supported
		route, err := util.CreateProxyRoute(ctx, failoverZone, apiExp.Spec.Zone, apiExp.Spec.ApiBasePath,
			contextutil.EnvFromContextOrDie(ctx),
			util.WithFailoverUpstreams(apiExposure.Spec.Upstreams...),
			util.WithFailoverSecurity(apiExposure.Spec.Security),
		)
		if err != nil {
			return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
		}
		apiExp.Status.FailoverRoute = types.ObjectRefFromObject(route)
	}

	apiExp.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))
	apiExp.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiExp.Status.Route = types.ObjectRefFromObject(route)
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
