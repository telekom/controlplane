// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"net/url"

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

	mapped := make([]api.ApplicationResponse, 0, len(items))
	for _, item := range items {
		resp := applicationmapper.MapResponse(item)
		if !matchesExternalIdFilters(&resp, &params) {
			continue
		}
		mapped = append(mapped, resp)
	}

	basePath := "/applications"
	if q := externalIdFilterQuery(&params); q != "" {
		basePath += "?" + q
	}
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

// matchesExternalIdFilters reports whether the application matches the
// optional icto/psiid query filters (exact match). Empty filters match all.
func matchesExternalIdFilters(app *api.ApplicationResponse, params *api.GetAllApplicationsParams) bool {
	if params.Icto != "" && app.Icto != params.Icto {
		return false
	}
	if params.Psiid != "" && app.Psiid != params.Psiid {
		return false
	}
	return true
}

// externalIdFilterQuery encodes the active icto/psiid filters so that
// pagination links keep them.
func externalIdFilterQuery(params *api.GetAllApplicationsParams) string {
	q := url.Values{}
	if params.Icto != "" {
		q.Set("icto", params.Icto)
	}
	if params.Psiid != "" {
		q.Set("psiid", params.Psiid)
	}
	return q.Encode()
}
