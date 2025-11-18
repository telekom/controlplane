// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ handler.Handler[*apiapi.ApiExposure] = (*ApiExposureHandler)(nil)

type ApiExposureHandler struct{}

func (h *ApiExposureHandler) CreateOrUpdate(ctx context.Context, apiExp *apiapi.ApiExposure) error {
	log := log.FromContext(ctx)

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
	if err := ApiExposureMustNotAlreadyExist(ctx, apiExp); err != nil {
		return errors.Wrapf(err, "failed to validate uniqueness of ApiExposure: %s in namespace: %s", apiExp.Name, apiExp.Namespace)
	}

	apiExp.SetCondition(NewApiExposureActiveCondition(apiExp, apiExp.Status.Active))
	if !apiExp.Status.Active {
		// its already exposed by another apiExposure, conditions are already set in the function
		return nil
	}

	// Core Validations are done, can continue

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
			util.WithFailoverUpstreams(apiExp.Spec.Upstreams...),
			util.WithFailoverSecurity(apiExp.Spec.Security),
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
