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
	eventsubscriptionmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/eventsubscription"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.EventSubscriptionController = (*eventSubscriptionController)(nil)

type eventSubscriptionController struct {
	stores *sstore.Stores
}

// NewEventSubscriptionController creates a new controller for EventSubscription read operations.
func NewEventSubscriptionController(stores *sstore.Stores) server.EventSubscriptionController {
	return &eventSubscriptionController{stores: stores}
}

func (c *eventSubscriptionController) Get(ctx context.Context, applicationId, eventSubscriptionName string) (api.EventSubscriptionResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.EventSubscriptionResponse{}, err
	}

	eventSubscriptionFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventSubscriptionName)

	sub, err := c.stores.EventSubscriptionStore.Get(ctx, appInfo.Namespace, eventSubscriptionFullName)
	if err != nil {
		return api.EventSubscriptionResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(sub, appInfo.AppName); err != nil {
		return api.EventSubscriptionResponse{}, err
	}

	return eventsubscriptionmapper.MapResponse(ctx, sub, c.stores), nil
}

func (c *eventSubscriptionController) GetAll(ctx context.Context, applicationId string, params api.GetAllEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error) {
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

	items, err := pagination.FetchAll(ctx, c.stores.EventSubscriptionStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.EventSubscriptionResponse, len(items))
	for i, item := range items {
		mapped[i] = eventsubscriptionmapper.MapResponse(ctx, item, c.stores)
	}

	basePath := fmt.Sprintf("/applications/%s/eventsubscriptions", applicationId)
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.EventSubscriptionListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *eventSubscriptionController) GetStatus(ctx context.Context, applicationId, eventSubscriptionName string) (api.ResourceStatusResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	eventSubscriptionFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventSubscriptionName)

	sub, err := c.stores.EventSubscriptionStore.Get(ctx, appInfo.Namespace, eventSubscriptionFullName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(sub, appInfo.AppName); err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(sub), nil
}
