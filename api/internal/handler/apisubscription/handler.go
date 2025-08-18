// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apisubscription/remote"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"

	"k8s.io/apimachinery/pkg/api/meta"
)

var _ handler.Handler[*apiapi.ApiSubscription] = (*ApiSubscriptionHandler)(nil)

type ApiSubscriptionHandler struct{}

func (h *ApiSubscriptionHandler) CreateOrUpdate(ctx context.Context, apiSub *apiapi.ApiSubscription) error {
	log := log.FromContext(ctx)

	scopedClient := cclient.ClientFromContextOrDie(ctx)

	if remote.IsRemoteApiSubscription(apiSub) {
		log.Info("ApiSubscription is remote")
		return remote.HandleRemoteApiSubscription(ctx, apiSub)
	}

	//  get corresponding active api
	exists, api, err := ApiMustExist(ctx, apiSub)
	if err != nil {
		return err
	}
	if !exists {

		log.Info("🧹 In this case we would also try to delete the child route")
		err = util.CleanupProxyRoute(ctx, apiSub.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}

		return nil
	}

	//  get corresponding active apiExposure
	exists, apiExposure, err := ApiExposureMustExist(ctx, apiSub)
	if err != nil {
		return err
	}
	if !exists {

		log.Info("🧹 In this case we would also try to delete the child route")
		err = util.CleanupProxyRoute(ctx, apiSub.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}

		return nil
	}

	// validate if basepathes of the api and apiexposure are really equal
	if api.Spec.BasePath != apiSub.Spec.ApiBasePath {
		return errors.Wrapf(err, "Subscriptions basePath: %s does not match the APIs basepath: %s",
			apiSub.Spec.ApiBasePath, api.Spec.BasePath)
	}

	// - validate visibility of apiExposure (WORLD, ENTERPRISE, ZONE) depending on subscription zone
	valid, err := ApiVisibilityMustBeValid(ctx, apiExposure, apiSub)
	if err != nil {
		return err
	}
	if !valid {
		apiSub.SetCondition(condition.NewNotReadyCondition("VisibilityConstraintViolation", "ApiExposure and ApiSubscription visibility combination is not allowed"))
		apiSub.SetCondition(condition.NewBlockedCondition(
			fmt.Sprintf("ApiSubscription is blocked. Subscriptions from zone '%s' are not allowed due to exposure visiblity constraints", apiSub.Spec.Zone.GetName())))
		return nil
	}

	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows subscription of api category

	// get application from cluster and get clientId from status
	application, err := util.GetApplication(ctx, apiSub.Spec.Requestor.Application)

	if err != nil {
		return errors.Wrapf(err,
			"unable to get application %s for Apisubscription", apiSub.Spec.Requestor.Application.String())
	}

	if meta.IsStatusConditionFalse(application.GetConditions(), condition.ConditionTypeReady) {
		apiSub.SetCondition(condition.NewNotReadyCondition("ApplicationNotReady", "Application was not yet processed"))
		apiSub.SetCondition(condition.NewBlockedCondition(
			"Application was not yet processed. ApiSubscription will be automatically processed, if the Application is ready"))

		log.Info("🧹 In this case we would also try to delete the child route")
		err = util.CleanupProxyRoute(ctx, apiSub.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}

		log.Info("❌ Application is not yet processed. ApiSubscription is blocked")

		return nil
	}

	// Approval

	requester := &approvalapi.Requester{
		Name:   application.Spec.Team,
		Email:  application.Spec.TeamEmail,
		Reason: fmt.Sprintf("Team %s requested access to your API %s from zone %s", application.Spec.Team, api.Name, apiSub.Spec.Zone.Name),
	}
	properties := map[string]any{
		"basePath": apiSub.Spec.ApiBasePath,
	}

	// Scopes
	// check if scopes exist and scopes are subset from api
	if apiSub.HasM2M() {
		if apiSub.Spec.Security.M2M.Scopes != nil {

			if len(api.Spec.Oauth2Scopes) == 0 {
				apiSub.SetCondition(condition.NewNotReadyCondition("ScopesNotDefined", "Api does not define any Oauth2 scopes"))
				apiSub.SetCondition(condition.NewBlockedCondition("Api does not define any Oauth2 scopes. ApiSubscription will be automatically processed, if the API will be updated with scopes"))
				return nil
			} else {
				scopesExist, invalidScopes := util.IsSubsetOfScopes(api.Spec.Oauth2Scopes, apiSub.Spec.Security.M2M.Scopes)
				if !scopesExist {
					var message = fmt.Sprintf("Some defined scopes are not available. Available scopes: \"%s\". Unsupported scopes: \"%s\"",
						strings.Join(api.Spec.Oauth2Scopes, ", "),
						strings.Join(invalidScopes, ", "),
					)
					apiSub.SetCondition(condition.NewNotReadyCondition("InvalidScopes", "One or more scopes which are defined in ApiSubscription are not defined in the ApiSpecification"))
					apiSub.SetCondition(condition.NewBlockedCondition(message))
					return nil
				}
			}

		}
		properties["scopes"] = apiSub.Spec.Security.M2M.Scopes
	}
	err = requester.SetProperties(properties)
	if err != nil {
		return errors.Wrapf(err, "unable to approvalRequest properties for apiSubscription: %s in namespace: %s",
			apiSub.Name, apiSub.Namespace)
	}

	approvalBuilder := builder.NewApprovalBuilder(scopedClient, apiSub)
	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(apiExposure.Spec.Approval.Strategy))

	if len(apiExposure.Spec.Approval.TrustedTeams) > 0 {
		approvalBuilder.WithTrustedRequesters(apiExposure.Spec.Approval.TrustedTeams)
	}

	res, err := approvalBuilder.Build(ctx)
	if err != nil {
		return err
	}

	apiSub.Status.ApprovalRequest = types.ObjectRefFromObject(approvalBuilder.GetApprovalRequest())
	apiSub.Status.Approval = types.ObjectRefFromObject(approvalBuilder.GetApproval())

	if res == builder.ApprovalResultDenied {
		log.Info("🧹 In this case we would delete the child resources")
		_, err := scopedClient.Cleanup(ctx, &gatewayapi.ConsumeRouteList{}, cclient.OwnedBy(apiSub))
		if err != nil {
			return errors.Wrapf(err, "Unable to cleanup consume routes for Apisubscription:  %s in namespace: %s",
				apiSub.Name, apiSub.Namespace)
		}

		err = util.CleanupProxyRoute(ctx, apiSub.Status.Route)
		if err != nil {
			return errors.Wrapf(err, "failed to delete route")
		}
		return nil
	}

	if res == builder.ApprovalResultPending {
		log.Info("🫷 Approval is pending and we will wait for it")
		return nil
	}

	log.Info("👌 Approval is granted and will continue with process")

	// ProxyRoute is only needed if subscriptionZone is different from exposureZone
	sameZoneAsExposure := apiSub.Spec.Zone.Equals(&apiExposure.Spec.Zone)
	// ProxyRoute is only needed if subscriptionZone is not used as failover zone
	failoverProxyRouteExists := apiExposure.HasFailover() && apiExposure.Spec.Traffic.Failover.ContainsZone(apiSub.Spec.Zone)

	options := []util.CreateRouteOption{}

	if sameZoneAsExposure || failoverProxyRouteExists {
		log.Info("Skipping creation of proxy route for ApiSubscription", "zone", apiSub.Spec.Zone)
		options = append(options, util.ReturnReferenceOnly())
	} else {
		log.Info("Creating proxy route for ApiSubscription", "zone", apiSub.Spec.Zone)
	}

	if apiExposure.HasFailover() {
		failoverZone := apiExposure.Spec.Traffic.Failover.Zones[0]
		options = append(options, util.WithFailoverZone(failoverZone))
	}

	proxyRoute, err := util.CreateProxyRoute(ctx, apiSub.Spec.Zone, apiExposure.Spec.Zone, apiSub.Spec.ApiBasePath,
		contextutil.EnvFromContextOrDie(ctx),
		options...,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to create proxy route for zone %s", apiSub.Spec.Zone.Name)
	}
	apiSub.Status.Route = types.ObjectRefFromObject(proxyRoute)

	consumeRoute, err := util.CreateConsumeRoute(ctx, apiSub, apiSub.Spec.Zone, *types.ObjectRefFromObject(proxyRoute), application.Status.ClientId)
	if err != nil {
		return errors.Wrapf(err, "failed to create normal ConsumeRoute")
	}
	apiSub.Status.ConsumeRoute = types.ObjectRefFromObject(consumeRoute)

	// ----- Failover -----

	apiSub.Status.FailoverRoutes = []types.ObjectRef{}
	apiSub.Status.FailoverConsumeRoutes = []types.ObjectRef{}
	if apiSub.HasFailover() {
		for _, subFailoverZone := range apiSub.Spec.Traffic.Failover.Zones {
			options := []util.CreateRouteOption{}
			if apiExposure.HasFailover() {
				if len(apiExposure.Spec.Traffic.Failover.Zones) != 1 {
					return errors.New("Must exactly define one failover zone")
				}
				expFailoverZone := apiExposure.Spec.Traffic.Failover.Zones[0]
				options = append(options, util.WithFailoverZone(expFailoverZone))
			}

			// Check if the failover zone is the same as the exposure failover zone, then there is no need to create a proxy route
			sameFailoverZoneAsExposureFailoverZone := apiExposure.HasFailover() && apiExposure.Spec.Traffic.Failover.ContainsZone(subFailoverZone)
			// Check if the failover zone is the same as the exposure zone, then there is no need to create a proxy route
			failoverZoneIsExposureZone := subFailoverZone.Equals(&apiExposure.Spec.Zone)

			if sameFailoverZoneAsExposureFailoverZone || failoverZoneIsExposureZone {
				log.Info("Skipping creation of proxy route for failover zone", "zone", subFailoverZone)
				options = append(options, util.ReturnReferenceOnly())
			} else {
				log.Info("Creating proxy route for failover zone", "zone", subFailoverZone)
			}

			failoverProxyRoute, err := util.CreateProxyRoute(ctx, subFailoverZone, apiExposure.Spec.Zone, apiSub.Spec.ApiBasePath,
				contextutil.EnvFromContextOrDie(ctx),
				options...,
			)
			if err != nil {
				return errors.Wrapf(err, "failed to create proxy route for zone %s in failover scenario", subFailoverZone)
			}
			apiSub.Status.FailoverRoutes = append(apiSub.Status.FailoverRoutes, *types.ObjectRefFromObject(failoverProxyRoute))

			log.Info("Creating failover ConsumeRoute for zone", "zone", subFailoverZone)
			consumeRoute, err = util.CreateConsumeRoute(ctx, apiSub, subFailoverZone, *types.ObjectRefFromObject(failoverProxyRoute), application.Status.ClientId)
			if err != nil {
				return errors.Wrapf(err, "failed to create failover ConsumeRoute for zone %s", subFailoverZone)
			}
			apiSub.Status.FailoverConsumeRoutes = append(apiSub.Status.FailoverConsumeRoutes, *types.ObjectRefFromObject(consumeRoute))
		}
	}

	// ---- Set Conditions ----
	apiSub.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiSub.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))

	return nil
}

func (h *ApiSubscriptionHandler) Delete(ctx context.Context, apiSub *apiapi.ApiSubscription) error {
	err := util.CleanupProxyRoute(ctx, apiSub.Status.Route)
	if err != nil {
		return errors.Wrapf(err, "failed to delete route")
	}

	for _, failoverRoute := range apiSub.Status.FailoverRoutes {
		err := util.CleanupProxyRoute(ctx, &failoverRoute)
		if err != nil {
			return errors.Wrapf(err, "failed to delete failover route")
		}
	}
	return nil
}
