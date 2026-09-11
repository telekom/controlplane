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
	eventexposuremapper "github.com/telekom/controlplane/discovery-server/internal/mapper/eventexposure"
	eventsubscriptionmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/eventsubscription"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.EventExposureController = (*eventExposureController)(nil)

type eventExposureController struct {
	stores *sstore.Stores
}

// NewEventExposureController creates a new controller for EventExposure read operations.
func NewEventExposureController(stores *sstore.Stores) server.EventExposureController {
	return &eventExposureController{stores: stores}
}

func (c *eventExposureController) Get(ctx context.Context, applicationId, eventExposureName string) (api.EventExposureResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.EventExposureResponse{}, err
	}

	eventExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventExposureName)

	exposure, err := c.stores.EventExposureStore.Get(ctx, appInfo.Namespace, eventExposureFullName)
	if err != nil {
		return api.EventExposureResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
		return api.EventExposureResponse{}, err
	}

	return eventexposuremapper.MapResponse(ctx, exposure, c.stores), nil
}

//nolint:dupl // type-specific list methods share structure but differ in types and stores
func (c *eventExposureController) GetAll(ctx context.Context, applicationId string, params api.GetAllEventExposuresParams) (*api.EventExposureListResponse, error) {
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

	items, err := pagination.FetchAll(ctx, c.stores.EventExposureStore, listOpts)
	if err != nil {
		return nil, err
	}

	mapped := make([]api.EventExposureResponse, len(items))
	for i, item := range items {
		mapped[i] = eventexposuremapper.MapResponse(ctx, item, c.stores)
	}

	basePath := fmt.Sprintf("/applications/%s/eventexposures", applicationId)
	result := pagination.Paginate(mapped, params.Offset, params.Limit, basePath)

	return &api.EventExposureListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}

func (c *eventExposureController) GetStatus(ctx context.Context, applicationId, eventExposureName string) (api.ResourceStatusResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	eventExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventExposureName)

	exposure, err := c.stores.EventExposureStore.Get(ctx, appInfo.Namespace, eventExposureFullName)
	if err != nil {
		return api.ResourceStatusResponse{}, err
	}

	if err := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); err != nil {
		return api.ResourceStatusResponse{}, err
	}

	return statusmapper.MapResponse(exposure), nil
}

func (c *eventExposureController) GetSubscriptions(ctx context.Context, applicationId, eventExposureName string, params api.GetAllExposureEventSubscriptionsParams) (*api.EventSubscriptionListResponse, error) {
	appInfo, err := mapper.ParseApplicationId(ctx, applicationId)
	if err != nil {
		return nil, err
	}

	eventExposureFullName := fmt.Sprintf("%s--%s", appInfo.AppName, eventExposureName)

	// Fetch the exposure and verify it belongs to this application.
	exposure, err := c.stores.EventExposureStore.Get(ctx, appInfo.Namespace, eventExposureFullName)
	if err != nil {
		return nil, err
	}
	if verifyErr := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); verifyErr != nil {
		return nil, verifyErr
	}

	// Fetch all event subscriptions (cross-namespace — no prefix, no app label filter)
	// and filter by eventType. Subscribers come from different teams/apps.
	listOpts := store.NewListOpts()
	allSubs, err := pagination.FetchAll(ctx, c.stores.EventSubscriptionStore, listOpts)
	if err != nil {
		return nil, err
	}

	targetEventType := exposure.Spec.EventType
	matchingSubs := make([]api.EventSubscriptionResponse, 0)
	for _, sub := range allSubs {
		if sub.Spec.EventType != targetEventType {
			continue
		}
		matchingSubs = append(matchingSubs, eventsubscriptionmapper.MapResponse(ctx, sub, c.stores))
	}

	basePath := fmt.Sprintf("/applications/%s/eventexposures/%s/eventsubscriptions", applicationId, eventExposureName)
	result := pagination.Paginate(matchingSubs, params.Offset, params.Limit, basePath)

	return &api.EventSubscriptionListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}, nil
}
