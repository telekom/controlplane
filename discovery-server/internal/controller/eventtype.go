// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"

	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	eventtypemapper "github.com/telekom/controlplane/discovery-server/internal/mapper/eventtype"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.EventTypeController = (*eventTypeController)(nil)

type eventTypeController struct {
	stores *sstore.Stores
}

// NewEventTypeController creates a new controller for EventType read operations.
func NewEventTypeController(stores *sstore.Stores) server.EventTypeController {
	return &eventTypeController{stores: stores}
}

func (c *eventTypeController) Get(ctx context.Context, eventTypeId string) (api.EventTypeResponse, error) {
	resourceInfo, err := mapper.ParseResourceId(ctx, eventTypeId)
	if err != nil {
		return api.EventTypeResponse{}, err
	}

	eventType, err := c.stores.EventTypeStore.Get(ctx, resourceInfo.Namespace, resourceInfo.Name)
	if err != nil {
		return api.EventTypeResponse{}, err
	}

	return eventtypemapper.MapResponse(eventType), nil
}

func (c *eventTypeController) GetAll(ctx context.Context, params api.GetAllEventTypesParams) (*api.EventTypeListResponse, error) {
	listOpts := store.NewListOpts()

	items, err := pagination.FetchAll(ctx, c.stores.EventTypeStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.EventTypeResponse, len(items))
	for i, item := range items {
		mapped[i] = eventtypemapper.MapResponse(item)
	}

	basePath := "/eventtypes"
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.EventTypeListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *eventTypeController) GetStatus(ctx context.Context, eventTypeName string) (api.ResourceStatusResponse, error) {
	resourceInfo, err := mapper.ParseResourceId(ctx, eventTypeName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	eventType, err := c.stores.EventTypeStore.Get(ctx, resourceInfo.Namespace, resourceInfo.Name)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(eventType), nil
}

// GetActive returns the single active EventType.
func (c *eventTypeController) GetActive(ctx context.Context, eventTypeName string) (api.EventTypeResponse, error) {
	listOpts := store.NewListOpts()

	isActive := store.Filter{
		Path:  eventtypemapper.EventTypeActiveJsonPath,
		Op:    store.OpEqual,
		Value: "true",
	}
	matchName := store.Filter{
		Path:  "metadata.name",
		Op:    store.OpEqual,
		Value: eventTypeName,
	}
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)
	listOpts.Filters = append(listOpts.Filters, isActive, matchName)

	res, err := c.stores.EventTypeStore.List(ctx, listOpts)
	if err != nil {
		return api.EventTypeResponse{}, err
	}

	if len(res.Items) == 0 {
		return api.EventTypeResponse{}, problems.NotFound(eventTypeName)
	}

	if len(res.Items) > 1 {
		return api.EventTypeResponse{}, problems.InternalServerError("Data inconsistency", "multiple active event types found")
	}

	resp := eventtypemapper.MapResponse(res.Items[0])
	return resp, nil
}
