// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/applicationinfo"
	"github.com/telekom/controlplane/rover-server/internal/mapper/rover/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/rover/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
)

var _ server.RoverController = &RoverController{}

type RoverController struct {
	Store store.ObjectStore[*roverv1.Rover]
}

func NewRoverController() *RoverController {
	return &RoverController{
		Store: s.RoverStore,
	}
}

// Create implements server.RoverController.
func (r *RoverController) Create(ctx context.Context, req api.RoverCreateRequest) (api.RoverResponse, error) {
	// Important Hint: This is a declarative API. The client should not create a rover, but only use the PUT method.
	// This is similar to how kubernetes works.
	// The main use case for the rover API will be to enable the usage of roverctl
	logr.FromContextOrDiscard(ctx).Info("Rover: Create not implemented", "rover", req)
	return api.RoverResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Delete implements server.RoverController.
func (r *RoverController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	err = r.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		return err
	}
	return nil
}

// Get implements server.RoverController.
func (r *RoverController) Get(ctx context.Context, resourceId string) (res api.RoverResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	rover, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	return out.MapRoverResponse(ctx, rover)
}

// GetAll implements server.RoverController.
func (r *RoverController) GetAll(ctx context.Context, params api.GetAllRoversParams) (*api.RoverListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.RoverResponse, 0, len(objList.Items))
	for _, r := range objList.Items {
		roverResponse, err := out.MapRoverResponse(ctx, r)
		if err != nil {
			return nil, problems.InternalServerError("Failed to map resource", err.Error())
		}
		list = append(list, roverResponse)
	}

	return &api.RoverListResponse{
		UnderscoreLinks: api.Links{
			Self: objList.Links.Self,
			Next: objList.Links.Next,
		},
		Items: list,
	}, nil
}

// Update implements server.RoverController.
func (r *RoverController) Update(ctx context.Context, resourceId string, req api.RoverUpdateRequest) (res api.RoverResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	obj, err := in.MapRequest(&req, id)
	if err != nil {
		return res, err
	}
	EnsureLabelsOrDie(ctx, obj)
	obj.Labels[config.BuildLabelKey("application")] = id.Name

	err = r.Store.CreateOrReplace(ctx, obj)
	if err != nil {
		return res, err
	}

	return r.Get(ctx, resourceId)
}

// GetStatus implements server.RoverController.
func (r *RoverController) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	rover, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	return status.MapRoverResponse(ctx, rover)
}

// GetApplicationInfo implements server.RoverController.
func (r *RoverController) GetApplicationInfo(ctx context.Context, resourceId string, params api.GetApplicationInfoParams) (res api.RoverInfoResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return res, problems.Forbidden("Security context not found", "Security context is required to evaluate permissions")
	}

	ns := id.Environment + "--" + id.Namespace
	rover, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	appInfo, err := applicationinfo.MapApplicationInfo(ctx, rover)
	if err != nil {
		return res, problems.InternalServerError("Failed to map resource", err.Error())
	}

	return api.RoverInfoResponse{
		Environment:  bCtx.Environment,
		Hub:          bCtx.Group,
		Team:         bCtx.Team,
		Applications: []api.ApplicationInfo{*appInfo},
	}, nil

}

// GetApplicationsInfo implements server.RoverController.
func (r *RoverController) GetApplicationsInfo(ctx context.Context, params api.GetApplicationsInfoParams) (res api.RoverInfoResponse, err error) {
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return res, problems.Forbidden("Security context not found", "Security context is required to evaluate permissions")
	}

	if bCtx.ClientType != security.ClientTypeTeam {
		return res, problems.BadRequest("Only team clients are allowed to get all applications")
	}

	listOpts := store.NewListOpts()
	store.EnforcePrefix(bCtx.Environment+"--"+bCtx.Group+"--"+bCtx.Team, &listOpts)
	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return res, err
	}

	list := make([]api.ApplicationInfo, 0, len(objList.Items))
	for _, r := range objList.Items {
		logr.FromContextOrDiscard(ctx).Info("GetApplicationsInfo", "name", r.Name)
		applicationInfo, err := applicationinfo.MapApplicationInfo(ctx, r)
		if err != nil {
			return res, problems.InternalServerError("Failed to map resource", err.Error())
		}
		list = append(list, *applicationInfo)
	}

	// TODO: Improvement item to validate organization and team with the organization api (double check)
	return api.RoverInfoResponse{
		Applications: list,
		Environment:  bCtx.Environment,
		Hub:          bCtx.Group,
		Team:         bCtx.Team,
	}, nil

}

func (r *RoverController) ResetRoverSecret(ctx context.Context, resourceId string) (res api.RoverSecretResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	rover, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		return res, problems.NotFound(resourceId, err.Error())
	}

	if rover == nil || rover.Status.Application == nil {
		return res, errors.New("rover resource is not processed and does not contain an application")
	}

	app, err := s.ApplicationStore.Get(ctx, rover.Status.Application.Namespace, rover.Status.Application.Name)
	if err != nil {
		return res, err
	}

	app.Spec.Secret = uuid.NewString()
	if err := s.ApplicationStore.CreateOrReplace(ctx, app); err != nil {
		return res, err
	}

	return api.RoverSecretResponse{
		Id:     app.Status.ClientId,
		Secret: app.Spec.Secret,
	}, nil

}
