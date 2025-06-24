// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"encoding/json"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apisubscription/remote"
	"github.com/telekom/controlplane/api/internal/handler/util"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	// TODO: further validations (currently contained in the old code)
	// - validate if team category allows subscription of api category
	// - validate visibility of apiExposure (WORLD, ENTERPRISE, ZONE) depending on subscription zone

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

	// Scopes
	// check if scopes exist and scopes are subset from api
	if apiSub.Spec.Security != nil && apiSub.Spec.Security.Authentication.OAuth2.Scopes != nil {
		scopesExist, err := ScopesMustExist(ctx, apiSub)
		if err != nil {
			return errors.Wrapf(err, "failed to check scopes for ApiSubscription: %s in namespace: %s",
				apiSub.Name, apiSub.Namespace)
		}
		if !scopesExist {
			log.Info("❌ One or more scopes which are defined in ApiSubscription are not defined in the Api")
			return nil
		}
	}

	// Approval

	requester := &approvalapi.Requester{ // TODO: get from somewhere (Team?)
		Name:   "Ron",
		Email:  "ron.gummich@telekom.de",
		Reason: "I need access to this API!!",
	}
	properties := map[string]any{
		"basePath": apiSub.Spec.ApiBasePath,
	}

	approvalBuilder := builder.NewApprovalBuilder(scopedClient, apiSub)

	approvalReq := approvalBuilder.GetApprovalRequest()

	// add scopes to approval, if scopes changed, update the approval
	if apiSub.Spec.Security != nil && apiSub.Spec.Security.Authentication.OAuth2.Scopes != nil {
		//check if approval request already exists
		if approvalReq.Spec.State != "" && approvalReq.Spec.State != approvalapi.ApprovalStatePending {
			// Check if existing scopes in the approval request are a subset of the new scopes
			if approvalReq.Spec.Requester.Properties.Raw != nil {
				var propertiesMap map[string]interface{}
				err := json.Unmarshal(approvalReq.Spec.Requester.Properties.Raw, &propertiesMap)
				if err != nil {
					return errors.Wrap(err, "failed to unmarshal approval request properties")
				}
				if propertiesMap["scopes"] != nil {
					existingScopes, ok := propertiesMap["scopes"].([]string)
					if ok {
						for _, scope := range existingScopes {
							if !slices.Contains(apiSub.Spec.Security.Authentication.OAuth2.Scopes, scope) {
								//scopes changed -> set ApprovalRequest to pending
								approvalReq.Spec.State = approvalapi.ApprovalStatePending
							}
						}
					} else {
						return errors.New("existing scopes in approval request are not of type []string")
					}
				}
			}

		}

		// Set the scopes in the properties
		properties["scopes"] = apiSub.Spec.Security.Authentication.OAuth2.Scopes
	}

	err = requester.SetProperties(properties)
	if err != nil {
		return errors.Wrapf(err, "unable to approvalRequest properties for apiSubscription: %s in namespace: %s",
			apiSub.Name, apiSub.Namespace)
	}

	approvalBuilder.WithHashValue(requester.Properties)
	approvalBuilder.WithRequester(requester)
	approvalBuilder.WithStrategy(approvalapi.ApprovalStrategy(apiExposure.Spec.Approval))

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

	// Route
	subscriptionZone, err := util.GetZone(ctx, scopedClient, apiSub.Spec.Zone.K8s())
	if err != nil {
		return err
	}

	route := &gatewayapi.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiSub.Spec.ApiBasePath),
			Namespace: subscriptionZone.Status.Namespace,
		},
	}

	// ProxyRoute is only needed if subscriptionZone is different from exposureZone
	if !apiSub.Spec.Zone.Equals(&apiExposure.Spec.Zone) {
		route, err = util.CreateProxyRoute(ctx, apiSub.Spec.Zone, apiExposure.Spec.Zone, apiSub.Spec.ApiBasePath, contextutil.EnvFromContextOrDie(ctx))
		if err != nil {
			return errors.Wrapf(err, "failed to create proxy route")
		}
	}

	apiSub.Status.Route = types.ObjectRefFromObject(route)

	routeConsumer := &gatewayapi.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiSub.Name,
			Namespace: apiSub.Namespace,
		},
	}

	mutate := func() error {
		if err := controllerutil.SetControllerReference(apiSub, routeConsumer, scopedClient.Scheme()); err != nil {
			return errors.Wrapf(err, "failed to set owner-reference on %v", routeConsumer)
		}
		routeConsumer.Labels = apiSub.Labels

		routeConsumer.Spec = gatewayapi.ConsumeRouteSpec{
			Route:        *types.ObjectRefFromObject(route),
			ConsumerName: application.Status.ClientId,
			Oauth2Scopes: apiSub.Spec.Security.Authentication.OAuth2.Scopes,
		}

		return nil
	}

	_, err = scopedClient.CreateOrUpdate(ctx, routeConsumer, mutate)
	if err != nil {
		return errors.Wrapf(err, "Unable to create consume route for Apisubscription:  %s in namespace: %s",
			apiSub.Name, apiSub.Namespace)
	}

	apiSub.Status.ConsumeRoute = types.ObjectRefFromObject(routeConsumer)
	apiSub.SetCondition(condition.NewDoneProcessingCondition("Successfully provisioned subresources"))
	apiSub.SetCondition(condition.NewReadyCondition("Provisioned", "Successfully provisioned subresources"))

	return nil
}

func (h *ApiSubscriptionHandler) Delete(ctx context.Context, apiSub *apiapi.ApiSubscription) error {
	err := util.CleanupProxyRoute(ctx, apiSub.Status.Route)
	if err != nil {
		return errors.Wrapf(err, "failed to delete route")
	}
	return nil
}
