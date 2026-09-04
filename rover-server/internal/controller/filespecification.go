// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	security "github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/filespecification/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/filespecification/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
)

var _ server.FileSpecificationController = &FileSpecificationController{}

type FileSpecificationController struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.FileSpecification]
}

func NewFileSpecificationController(stores *s.Stores) *FileSpecificationController {
	return &FileSpecificationController{
		stores: stores,
		Store:  stores.FileSpecificationStore,
	}
}

// Create implements server.FileSpecificationController.
// This is a declarative API — clients should use PUT (Update) instead.
func (f *FileSpecificationController) Create(ctx context.Context, req api.FileSpecificationCreateRequest) (api.FileSpecificationResponse, error) {
	log.Infof("FileSpecification: Create not implemented. FileSpecification is: %+v", req)
	return api.FileSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

func (f *FileSpecificationController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	err = f.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}
	return nil
}

func (f *FileSpecificationController) Get(ctx context.Context, resourceId string) (res api.FileSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	fileSpec, err := f.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return out.MapResponse(fileSpec)
}

func (f *FileSpecificationController) GetAll(ctx context.Context, params api.GetAllFileSpecificationsParams) (*api.FileSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := f.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.FileSpecificationResponse, 0, len(objList.Items))
	for _, fileSpec := range objList.Items {
		resp, err := out.MapResponse(fileSpec)
		if err != nil {
			return nil, problems.InternalServerError("Failed to map resource", err.Error())
		}
		list = append(list, resp)
	}

	return &api.FileSpecificationListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

func (f *FileSpecificationController) Update(ctx context.Context, resourceId string, req api.FileSpecification) (res api.FileSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	fileSpec, err := in.MapRequest(req, id)
	if err != nil {
		return res, problems.BadRequest(err.Error())
	}
	EnsureLabelsOrDie(ctx, fileSpec)

	err = f.Store.CreateOrReplace(ctx, fileSpec)
	if err != nil {
		return res, err
	}

	return f.Get(ctx, resourceId)
}

func (f *FileSpecificationController) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	fileSpec, err := f.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapResponse(ctx, fileSpec)
}
