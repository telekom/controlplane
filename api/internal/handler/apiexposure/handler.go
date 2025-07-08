// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	err = createRealRoute(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "unable to create real route for apiExposure: %s in namespace: %s", obj.Name, obj.Namespace)
	}

	obj.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))
	obj.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
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

	return nil
}

// createRealRoute creates a real route for the given apiExposure
func createRealRoute(ctx context.Context, obj *apiapi.ApiExposure) error {
	scopedClient := cclient.ClientFromContextOrDie(ctx)
	envName := contextutil.EnvFromContextOrDie(ctx)

	// get referenced Zone from exposure
	zone, err := util.GetZone(ctx, scopedClient, obj.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Unable to get zone from apiExposure:  %s in namespace: %s", obj.Name, obj.Namespace))
	}

	downstreamRealm, err := util.GetRealm(ctx, client.ObjectKey{Name: envName, Namespace: zone.Status.Namespace})
	if err != nil {
		return err
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(obj.Spec.ApiBasePath),
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		route.Labels = map[string]string{
			apiapi.BasePathLabelKey:       labelutil.NormalizeValue(obj.Spec.ApiBasePath),
			config.BuildLabelKey("zone"):  labelutil.NormalizeValue(zone.Name),
			config.BuildLabelKey("realm"): labelutil.NormalizeValue(downstreamRealm.Name),
			config.BuildLabelKey("type"):  "real",
		}

		downstream, err := downstreamRealm.AsDownstream(obj.Spec.ApiBasePath)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		upstream, err := util.AsUpstreamForRealRoute(ctx, obj)
		if err != nil {
			return errors.Wrap(err, "failed to create downstream")
		}

		route.Spec = gatewayapi.RouteSpec{
			Realm: *types.ObjectRefFromObject(downstreamRealm),
			Upstreams: []gatewayapi.Upstream{
				upstream,
			},
			Downstreams: []gatewayapi.Downstream{
				downstream,
			},
		}

		if obj.Spec.HasExternalIdp() {
			route.Spec.Security = &gatewayapi.Security{
				M2M: &gatewayapi.Machine2MachineAuthentication{
					ExternalIDP: &gatewayapi.ExternalIdentityProvider{
						TokenEndpoint: obj.Spec.Security.M2M.ExternalIDP.TokenEndpoint,
						TokenRequest:  obj.Spec.Security.M2M.ExternalIDP.TokenRequest,
						GrantType:     obj.Spec.Security.M2M.ExternalIDP.GrantType,
						Client: &gatewayapi.OAuth2ClientCredentials{
							ClientId:     obj.Spec.Security.M2M.ExternalIDP.Client.ClientId,
							ClientSecret: obj.Spec.Security.M2M.ExternalIDP.Client.ClientSecret,
							Scopes:       obj.Spec.Security.M2M.ExternalIDP.Client.Scopes,
						},
					},
				},
			}
		}

		return nil
	}

	_, err = scopedClient.CreateOrUpdate(ctx, route, mutator)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update route: %s in namespace: %s", route.Name, route.Namespace)
	}

	obj.Status.Route = &types.ObjectRef{
		Name:      route.Name,
		Namespace: route.Namespace,
	}

	return nil
}
