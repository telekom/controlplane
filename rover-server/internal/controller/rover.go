// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/applicationinfo"
	"github.com/telekom/controlplane/rover-server/internal/mapper/rover/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/rover/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ server.RoverController = &RoverController{}

type RoverController struct {
	stores      *s.Stores
	Store       store.ObjectStore[*roverv1.Rover]
	SecretStore store.ObjectStore[*roverv1.Rover]
}

func NewRoverController(stores *s.Stores) *RoverController {
	return &RoverController{
		stores:      stores,
		Store:       stores.RoverStore,
		SecretStore: stores.RoverSecretStore,
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
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
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
	rover, err := r.SecretStore.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return out.MapResponse(ctx, rover, r.stores)
}

// GetAll implements server.RoverController.
func (r *RoverController) GetAll(ctx context.Context, params api.GetAllRoversParams) (*api.RoverListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := r.SecretStore.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.RoverResponse, 0, len(objList.Items))
	for _, item := range objList.Items {
		roverResponse, err := out.MapResponse(ctx, item, r.stores)
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

	if err := r.guardPubSubFeature(ctx, req, config.FeaturePubSub.IsEnabled()); err != nil {
		return res, err
	}

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
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapRoverResponse(ctx, rover, r.stores)
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
	rover, err := r.SecretStore.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	appInfo, err := applicationinfo.MapApplicationInfo(ctx, rover, r.stores)
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
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)
	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return res, err
	}

	// Build a set of requested names for efficient lookup
	nameFilter := make(map[string]struct{}, len(params.Names))
	for _, n := range params.Names {
		nameFilter[n] = struct{}{}
	}

	list := make([]api.ApplicationInfo, 0, len(objList.Items))
	for _, rover := range objList.Items {
		// If names filter is provided, skip rovers not in the list
		if len(nameFilter) > 0 {
			if _, ok := nameFilter[rover.Name]; !ok {
				continue
			}
		}
		logr.FromContextOrDiscard(ctx).Info("GetApplicationsInfo", "name", rover.Name)
		applicationInfo, err := applicationinfo.MapApplicationInfo(ctx, rover, r.stores)
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
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	newClientSecret, err := secrets.GenerateSecret()
	if err != nil {
		return res, problems.InternalServerError("Failed to generate new client secret", err.Error())
	}
	rover.Spec.ClientSecret = newClientSecret
	if err := r.Store.CreateOrReplace(ctx, rover); err != nil {
		return res, err
	}

	if rover.Status.Application == nil {
		return res, problems.BadRequest("Application not found or not fully processed. Try again later.")
	}
	app, err := r.stores.ApplicationStore.Get(ctx, rover.Status.Application.Namespace, rover.Status.Application.Name)
	if err != nil {
		return res, err
	}

	return api.RoverSecretResponse{
		Id:     app.Status.ClientId,
		Secret: newClientSecret,
	}, nil
}

func (r *RoverController) guardPubSubFeature(ctx context.Context, res api.RoverUpdateRequest, isEnabled bool) problems.Problem {
	if isEnabled {
		return nil
	}

	fields := []problems.Field{}

	for i, e := range res.Exposures {
		d, err := e.Discriminator()
		if err != nil {
			continue
		}
		if d == "event" {
			fields = append(fields, problems.Field{
				Field:  fmt.Sprintf("exposures[%d]", i),
				Detail: "Pub/Sub features are not enabled, but the request contains an event exposure",
			})
		}
	}

	for i, s := range res.Subscriptions {
		d, err := s.Discriminator()
		if err != nil {
			continue
		}
		if d == "event" {
			fields = append(fields, problems.Field{
				Field:  fmt.Sprintf("exposures[%d]", i),
				Detail: "Pub/Sub features are not enabled, but the request contains an event exposure",
			})
		}
	}

	if len(fields) > 0 {
		msg := "The request contains Pub/Sub features, but this feature is not enabled on the server."
		return problems.Builder().Detail(msg).Title("Feature has not been enabled").Status(400).Fields(fields...).Build()
	}

	return nil
}
