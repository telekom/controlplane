// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"

	"github.com/telekom/controlplane/spy-server/internal/api"
	"github.com/telekom/controlplane/spy-server/internal/mapper"
	exposuremapper "github.com/telekom/controlplane/spy-server/internal/mapper/apiexposure"
	subscriptionmapper "github.com/telekom/controlplane/spy-server/internal/mapper/apisubscription"
	statusmapper "github.com/telekom/controlplane/spy-server/internal/mapper/status"
	"github.com/telekom/controlplane/spy-server/internal/pagination"
	"github.com/telekom/controlplane/spy-server/internal/server"
	sstore "github.com/telekom/controlplane/spy-server/pkg/store"
)

// Compile-time interface check.
var _ server.ApiExposureController = (*apiExposureController)(nil)

type apiExposureController struct {
	stores *sstore.Stores
}

// NewApiExposureController creates a new controller for ApiExposure read operations.
func NewApiExposureController(stores *sstore.Stores) server.ApiExposureController {
	return &apiExposureController{stores: stores}
}

func (c *apiExposureController) Get(ctx context.Context, applicationId, apiExposureName string) (api.ApiExposureResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ApiExposureResponse{}, err
	}

	apiExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, apiExposureName)

	exposure, err := c.stores.APIExposureStore.Get(ctx, appInfo.Namespace, apiExposureFullName)
	if err != nil {
		return api.ApiExposureResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
		return api.ApiExposureResponse{}, err
	}

	return exposuremapper.MapResponse(exposure), nil
}

func (c *apiExposureController) GetAll(ctx context.Context, applicationId string, params api.GetAllApiExposuresParams) (*api.ApiExposureListResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return nil, err
	}

	listOpts := store.NewListOpts()
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)
	listOpts.Prefix = appInfo.Namespace + "/"
	listOpts.Filters = append(listOpts.Filters, store.Filter{
		Path:  mapper.ApplicationLabelPath,
		Op:    store.OpEqual,
		Value: appInfo.AppName,
	})

	items, err := pagination.FetchAll(ctx, c.stores.APIExposureStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.ApiExposureResponse, len(items))
	for i, item := range items {
		mapped[i] = exposuremapper.MapResponse(item)
	}

	basePath := fmt.Sprintf("/applications/%s/apiexposures", applicationId)
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.ApiExposureListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *apiExposureController) GetStatus(ctx context.Context, applicationId, apiExposureName string) (api.ResourceStatusResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	apiExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, apiExposureName)

	exposure, err := c.stores.APIExposureStore.Get(ctx, appInfo.Namespace, apiExposureFullName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(exposure), nil
}

func (c *apiExposureController) GetSubscriptions(ctx context.Context, applicationId, apiExposureName string, params api.GetAllExposureApiSubscriptionsParams) (*api.ApiSubscriptionListResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return nil, err
	}

	apiExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, apiExposureName)

	// Fetch the exposure and verify it belongs to this application.
	exposure, err := c.stores.APIExposureStore.Get(ctx, appInfo.Namespace, apiExposureFullName)
	if err != nil {
		return nil, err
	}
	if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
		return nil, err
	}

	// Fetch all subscriptions (cross-namespace — no prefix, no app label filter)
	// and filter by basePath. Subscribers come from different teams/apps.
	listOpts := store.NewListOpts()
	allSubs, err := pagination.FetchAll(ctx, c.stores.APISubscriptionStore, listOpts)
	if err != nil {
		return nil, err
	}

	targetBasePath := exposure.Spec.ApiBasePath
	matchingSubs := make([]api.ApiSubscriptionResponse, 0)
	for _, sub := range allSubs {
		if sub.Spec.ApiBasePath != targetBasePath {
			continue
		}
		matchingSubs = append(matchingSubs, subscriptionmapper.MapResponse(ctx, sub, c.stores))
	}

	basePath := fmt.Sprintf("/applications/%s/apiexposures/%s/apisubscriptions", applicationId, apiExposureName)
	result := pagination.Paginate(matchingSubs, params.Offset, params.Limit, basePath)

	return &api.ApiSubscriptionListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}
