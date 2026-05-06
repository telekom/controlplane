// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	applicationmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/application"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.ApplicationController = (*applicationController)(nil)

type applicationController struct {
	stores *sstore.Stores
}

// NewApplicationController creates a new controller for Application read operations.
func NewApplicationController(stores *sstore.Stores) server.ApplicationController {
	return &applicationController{stores: stores}
}

func (c *applicationController) GetAll(ctx context.Context, params api.GetAllApplicationsParams) (*api.ApplicationListResponse, error) {
	listOpts := store.NewListOpts()
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	items, err := pagination.FetchAll(ctx, c.stores.ApplicationStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.ApplicationResponse, len(items))
	for i, item := range items {
		mapped[i] = applicationmapper.MapResponse(item)
	}

	basePath := "/applications"
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.ApplicationListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *applicationController) Get(ctx context.Context, applicationId string) (api.ApplicationResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ApplicationResponse{}, err
	}

	app, err := c.stores.ApplicationStore.Get(ctx, appInfo.Namespace, appInfo.AppName)
	if err != nil {
		return api.ApplicationResponse{}, err
	}

	return applicationmapper.MapResponse(app), nil
}

func (c *applicationController) GetStatus(ctx context.Context, applicationId string) (api.ResourceStatusResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	app, err := c.stores.ApplicationStore.Get(ctx, appInfo.Namespace, appInfo.AppName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(app), nil
}
