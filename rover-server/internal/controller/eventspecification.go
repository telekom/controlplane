// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	security "github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/eventspecification/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/eventspecification/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
)

var _ server.EventSpecificationController = &EventSpecificationController{}

type EventSpecificationController struct {
	Store store.ObjectStore[*roverv1.EventSpecification]
}

func NewEventSpecificationController() *EventSpecificationController {
	return &EventSpecificationController{
		Store: s.EventSpecificationStore,
	}
}

// Create implements server.EventSpecificationController.
// This is a declarative API — clients should use PUT (Update) instead.
func (e *EventSpecificationController) Create(ctx context.Context, req api.EventSpecificationCreateRequest) (api.EventSpecificationResponse, error) {
	log.Infof("EventSpecification: Create not implemented. EventSpecification is: %+v", req)
	return api.EventSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Delete implements server.EventSpecificationController.
func (e *EventSpecificationController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	if cconfig.FeatureFileManager.IsEnabled() {
		// Delete the optional specification file from file-manager
		fileId := generateFileId(id)
		err = file.GetFileManager().DeleteFile(ctx, fileId)
		if err != nil {
			if !errors.Is(err, file.ErrNotFound) {
				return err
			}
			// File not found is acceptable — specification is optional
		}
	}

	ns := id.Environment + "--" + id.Namespace
	err = e.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}
	return nil
}

// Get implements server.EventSpecificationController.
func (e *EventSpecificationController) Get(ctx context.Context, resourceId string) (res api.EventSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	eventSpec, err := e.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	var specContent map[string]any
	specContent, err = e.downloadSpecification(ctx, eventSpec.Spec.Specification)
	if err != nil {
		return res, err
	}

	return out.MapResponse(eventSpec, specContent)
}

// GetAll implements server.EventSpecificationController.
func (e *EventSpecificationController) GetAll(ctx context.Context, params api.GetAllEventSpecificationsParams) (*api.EventSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := e.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.EventSpecificationResponse, 0, len(objList.Items))
	for _, eventSpec := range objList.Items {
		specContent, err := e.downloadSpecification(ctx, eventSpec.Spec.Specification)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download resource", err.Error())
		}

		resp, err := out.MapResponse(eventSpec, specContent)
		if err != nil {
			return nil, problems.InternalServerError("Failed to map resource", err.Error())
		}
		list = append(list, resp)
	}

	return &api.EventSpecificationListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

// Update implements server.EventSpecificationController.
func (e *EventSpecificationController) Update(ctx context.Context, resourceId string, req api.EventSpecification) (res api.EventSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	// Handle the optional specification payload
	var specOrFileId string
	if len(req.Specification) > 0 {
		specMarshaled, marshalErr := json.Marshal(req.Specification)
		if marshalErr != nil {
			return res, problems.BadRequest(marshalErr.Error())
		}

		uploadRes, err := e.uploadFile(ctx, specMarshaled, id)
		if err != nil {
			return res, err
		}
		if uploadRes != nil {
			specOrFileId = uploadRes.FileId
		}
	}

	eventSpec, err := in.MapRequest(req, specOrFileId, id)
	if err != nil {
		return res, problems.BadRequest(err.Error())
	}
	EnsureLabelsOrDie(ctx, eventSpec)

	err = e.Store.CreateOrReplace(ctx, eventSpec)
	if err != nil {
		return res, err
	}

	return e.Get(ctx, resourceId)
}

// GetStatus implements server.EventSpecificationController.
func (e *EventSpecificationController) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	eventSpec, err := e.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapResponse(ctx, eventSpec)
}

func (e *EventSpecificationController) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (res *filesapi.FileUploadResponse, err error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil, nil
	}

	fileId := generateFileId(id)
	fileContentType := "application/json"
	return file.GetFileManager().UploadFile(ctx, fileId, fileContentType, bytes.NewReader(specMarshaled))
}

// downloadSpecification retrieves the optional specification file content.
// Returns nil if no specification is stored (fileId is empty).
func (e *EventSpecificationController) downloadSpecification(ctx context.Context, fileId string) (map[string]any, error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil, nil
	}

	if fileId == "" {
		return nil, nil
	}

	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(&b)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	m := make(map[string]any)
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}

	return m, nil
}
