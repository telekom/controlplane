// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	exposuremapper "github.com/telekom/controlplane/discovery-server/internal/mapper/apiexposure"
	subscriptionmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/apisubscription"
	statusmapper "github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	"github.com/telekom/controlplane/discovery-server/internal/pagination"
	"github.com/telekom/controlplane/discovery-server/internal/server"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// Compile-time interface check.
var _ server.ApiExposureController = (*apiExposureController)(nil)

// Projected JSON paths mask existing fields without creating missing credentials.
var subscriberCredentialPaths = []string{
	"items.#.security.username",
	"items.#.security.password",
	"items.#.security.clientId",
	"items.#.security.clientSecret",
	"items.#.security.refreshToken",
	"items.#.security.rotateToken",
}

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

	exposure, err := c.stores.APIExposureSecretStore.Get(ctx, appInfo.Namespace, apiExposureFullName)
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

	items, err := pagination.FetchAll(ctx, c.stores.APIExposureSecretStore, listOpts)
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
	if verifyErr := mapper.VerifyApplicationLabel(exposure, appInfo.AppName); verifyErr != nil {
		return nil, verifyErr
	}

	// Subscribers come from different teams/apps, so only the basePath filter applies.
	// Exposure access does not authorize disclosure of subscriber credentials:
	// the plain store is used so no secrets are resolved, and every response is
	// masked regardless of the caller's scope.
	listOpts := store.NewListOpts()
	listOpts.Filters = append(listOpts.Filters, store.Filter{
		Path:  "spec.apiBasePath",
		Op:    store.OpEqual,
		Value: exposure.Spec.ApiBasePath,
	})
	subs, err := pagination.FetchAll(ctx, c.stores.APISubscriptionStore, listOpts)
	if err != nil {
		return nil, err
	}

	matchingSubs := make([]api.ApiSubscriptionResponse, len(subs))
	for i, sub := range subs {
		matchingSubs[i] = subscriptionmapper.MapResponse(ctx, sub, c.stores)
	}

	basePath := fmt.Sprintf("/applications/%s/apiexposures/%s/apisubscriptions", applicationId, apiExposureName)
	result := pagination.Paginate(matchingSubs, params.Offset, params.Limit, basePath)

	resp := &api.ApiSubscriptionListResponse{
		Items:           result.Items,
		Paging:          result.Paging,
		UnderscoreLinks: result.Links,
	}
	if _, err = secrets.NewObfuscator().ReplaceAll(ctx, resp, subscriberCredentialPaths); err != nil {
		return nil, problems.InternalServerError("Internal Server Error", "failed to mask subscriber credentials")
	}
	return resp, nil
}
