// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	subscriptionmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/apisubscription"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.ApiSubscriptionController = (*apiSubscriptionController)(nil)

type apiSubscriptionController struct {
	stores *sstore.Stores
}

// NewApiSubscriptionController creates a new controller for ApiSubscription read operations.
func NewApiSubscriptionController(stores *sstore.Stores) server.ApiSubscriptionController {
	return &apiSubscriptionController{stores: stores}
}

func (c *apiSubscriptionController) Get(ctx context.Context, applicationId, apiSubscriptionName string) (api.ApiSubscriptionResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ApiSubscriptionResponse{}, err
	}

	apiSubscriptionFullName := fmt.Sprintf("%s--%s", appInfo.AppName, apiSubscriptionName)

	sub, err := c.stores.APISubscriptionSecretStore.Get(ctx, appInfo.Namespace, apiSubscriptionFullName)
	if err != nil {
		return api.ApiSubscriptionResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(sub, appInfo.AppName); err != nil {
		return api.ApiSubscriptionResponse{}, err
	}

	return subscriptionmapper.MapResponse(ctx, sub, c.stores), nil
}

func (c *apiSubscriptionController) GetAll(ctx context.Context, applicationId string, params api.GetAllApiSubscriptionsParams) (*api.ApiSubscriptionListResponse, error) {
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

	items, err := pagination.FetchAll(ctx, c.stores.APISubscriptionSecretStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.ApiSubscriptionResponse, len(items))
	for i, item := range items {
		mapped[i] = subscriptionmapper.MapResponse(ctx, item, c.stores)
	}

	basePath := fmt.Sprintf("/applications/%s/apisubscriptions", applicationId)
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.ApiSubscriptionListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *apiSubscriptionController) GetStatus(ctx context.Context, applicationId, apiSubscriptionName string) (api.ResourceStatusResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	apiSubscriptionFullName := fmt.Sprintf("%s--%s", appInfo.AppName, apiSubscriptionName)

	sub, err := c.stores.APISubscriptionStore.Get(ctx, appInfo.Namespace, apiSubscriptionFullName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(sub, appInfo.AppName); err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(sub), nil
}
