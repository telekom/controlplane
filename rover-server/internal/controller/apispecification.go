// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/apispecification/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/apispecification/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
)

var _ server.ApiSpecificationController = &ApiSpecificationController{}

type ApiSpecificationController struct {
	Store store.ObjectStore[*roverv1.ApiSpecification]
}

func NewApiSpecificationController() *ApiSpecificationController {
	return &ApiSpecificationController{
		Store: s.ApiSpecificationStore,
	}
}

// Create implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Create(ctx context.Context, req api.ApiSpecificationCreateRequest) (res api.ApiSpecificationResponse, err error) {
	// Important Hint: This is a declarative API. The client should not create an ApiSpecification, but only use
	// the PUT method. This is similar to how kubernetes works.
	// The main use case for the rover API will be to enable the usage of roverctl
	log.Infof("ApiSpecification: Create not implemented. ApiSpecification is: %+v", req)
	return api.ApiSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Delete implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	err = a.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		notFound := errors.IsNotFound(err)
		if notFound {
			return problems.NotFound(resourceId, err.Error())
		}

		return problems.InternalServerError("Failed to delete resource", err.Error())
	}
	return nil
}

// Get implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Get(ctx context.Context, resourceId string) (res api.ApiSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	apiSpec, err := a.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	return out.MapResponse(apiSpec)
}

// GetAll implements server.ApiSpecificationController.
func (a *ApiSpecificationController) GetAll(ctx context.Context, params api.GetAllApiSpecificationsParams) (*api.ApiSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor

	objList, err := a.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.ApiSpecificationResponse, 0, len(objList.Items))
	for _, apispec := range objList.Items {
		resp, err := out.MapResponse(apispec)
		if err != nil {
			return nil, problems.InternalServerError("Failed to map resource", err.Error())
		}
		list = append(list, resp)
	}

	return &api.ApiSpecificationListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

// Update implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Update(ctx context.Context, resourceId string, req api.ApiSpecification) (res api.ApiSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}
	obj, err := in.MapRequest(&req, id)
	if err != nil {
		return res, err
	}
	EnsureLabelsOrDie(ctx, obj)

	err = a.Store.CreateOrReplace(ctx, obj)
	if err != nil {
		return res, err
	}

	return a.Get(ctx, resourceId)
}

// GetStatus implements server.ApiSpecificationController.
func (a *ApiSpecificationController) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	apiSpec, err := a.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	return status.MapResponse(apiSpec.Status.Conditions)
}
