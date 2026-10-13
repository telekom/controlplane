// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	security "github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"

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
	if err := f.deleteFile(ctx, ns, id.Name); err != nil {
		return err
	}

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

	specContent, err := f.downloadFile(ctx, fileSpec.Spec.Specification)
	if err != nil {
		return res, err
	}
	return out.MapResponse(fileSpec, specContent)
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
		specContent, err := f.downloadFile(ctx, fileSpec.Spec.Specification)
		if err != nil {
			return nil, err
		}

		resp, err := out.MapResponse(fileSpec, specContent)
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

	var specOrFileId string
	if len(req.Specification) > 0 {
		specMarshaled, marshalErr := yaml.Marshal(req.Specification)
		if marshalErr != nil {
			return res, problems.BadRequest(marshalErr.Error())
		}

		uploadRes, err := f.uploadFile(ctx, specMarshaled, id)
		if err != nil {
			return res, err
		}

		specOrFileId = uploadRes.FileId
	} else if err := f.deleteFile(ctx, id.Environment+"--"+id.Namespace, id.Name); err != nil {
		return res, err
	}

	fileSpec, err := in.MapRequest(req, specOrFileId, id)
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

func (f *FileSpecificationController) deleteFile(ctx context.Context, ns, name string) error {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil
	}

	fileId, err := f.existingFileId(ctx, ns, name)
	if err != nil || fileId == "" {
		return err
	}

	err = file.GetFileManager().DeleteFile(ctx, fileId)
	if err != nil && !errors.Is(err, file.ErrNotFound) {
		return err
	}
	return nil
}

func (f *FileSpecificationController) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (res *filesapi.FileUploadResponse, err error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return &filesapi.FileUploadResponse{}, nil
	}

	existingId, err := f.existingFileId(ctx, id.Environment+"--"+id.Namespace, id.Name)
	if err != nil {
		return nil, err
	}
	fileId, err := resolveFileId(existingId)
	if err != nil {
		return nil, err
	}

	fileContentType := "application/yaml"
	return file.GetFileManager().UploadFile(ctx, fileId, fileContentType, bytes.NewReader(specMarshaled))
}

// downloadFile retrieves the optional specification file content.
// Returns nil if no specification is stored (fileId is empty).
func (f *FileSpecificationController) downloadFile(ctx context.Context, fileId string) (map[string]any, error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil, nil
	}

	if fileId == "" {
		return nil, nil
	}

	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, problems.InternalServerError("Failed to download file specification", err.Error())
	}

	if b.Len() == 0 {
		return nil, nil
	}

	var specContent map[string]any
	if err := yaml.NewDecoder(&b).Decode(&specContent); err != nil {
		return nil, problems.InternalServerError("Failed to unmarshal file specification", err.Error())
	}
	return specContent, nil
}

// existingFileId returns the stored specification file ID, or "" if the resource does not exist.
func (f *FileSpecificationController) existingFileId(ctx context.Context, ns, name string) (string, error) {
	fileSpec, err := f.Store.Get(ctx, ns, name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return fileSpec.Spec.Specification, nil
}
