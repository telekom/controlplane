// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	"github.com/telekom/controlplane/rover-server/internal/mapper/roadmap/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/roadmap/out"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

type RoadmapController struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.Roadmap]
}

func NewRoadmapController(stores *s.Stores) *RoadmapController {
	return &RoadmapController{
		stores: stores,
		Store:  stores.RoadmapStore,
	}
}

// Create implements ApiRoadmapController.
func (r *RoadmapController) Create(ctx context.Context, req api.ApiRoadmapCreateRequest) (api.ApiRoadmapResponse, error) {
	log.Infof("ApiRoadmap: Create not implemented. ApiRoadmap is: %+v", req)
	return api.ApiRoadmapResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Update implements ApiRoadmapController.
func (r *RoadmapController) Update(ctx context.Context, resourceId string, req api.ApiRoadmapUpdateRequest) (api.ApiRoadmapResponse, error) {
	var res api.ApiRoadmapResponse

	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	if req.BasePath == "" {
		return res, problems.BadRequest("basePath must not be empty")
	}
	if len(req.Items) == 0 {
		return res, problems.BadRequest("items array must contain at least one item")
	}

	expectedName := in.MakeRoadmapName(req.BasePath)
	if expectedName != id.Name {
		return res, problems.BadRequest("basePath " + req.BasePath + " does not match resource ID " + resourceId)
	}

	return r.createOrUpdateRoadmap(ctx, id, req.BasePath, req.Items)
}

func (r *RoadmapController) createOrUpdateRoadmap(ctx context.Context, id mapper.ResourceIdInfo, basePath string, items []api.ApiRoadmapItem) (api.ApiRoadmapResponse, error) {
	var res api.ApiRoadmapResponse

	itemsMarshaled, err := json.Marshal(items)
	if err != nil {
		return res, problems.BadRequest("failed to marshal items: " + err.Error())
	}

	fileAPIResp, err := r.uploadFile(ctx, itemsMarshaled, id)
	if err != nil {
		return res, err
	}

	roadmap, err := in.MapRequest(basePath, fileAPIResp, id)
	if err != nil {
		return res, err
	}
	EnsureLabelsOrDie(ctx, roadmap)

	err = r.Store.CreateOrReplace(ctx, roadmap)
	if err != nil {
		return res, err
	}

	return out.MapResponse(roadmap, items), nil
}

// Get implements ApiRoadmapController.
func (r *RoadmapController) Get(ctx context.Context, resourceId string) (api.ApiRoadmapResponse, error) {
	var res api.ApiRoadmapResponse

	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	roadmap, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	// Download items from file-manager
	reader, err := r.downloadFile(ctx, roadmap.Spec.Contents)
	if err != nil {
		return res, errors.Wrap(err, "failed to download roadmap items from file-manager")
	}

	var items []api.ApiRoadmapItem
	err = json.NewDecoder(reader).Decode(&items)
	if err != nil {
		return res, errors.Wrap(err, "failed to decode roadmap items")
	}

	return out.MapResponse(roadmap, items), nil
}

// GetAll implements ApiRoadmapController.
func (r *RoadmapController) GetAll(ctx context.Context, params api.GetAllApiRoadmapsParams) (*api.ApiRoadmapListResponse, error) {
	listOpts := store.NewListOpts()
	if params.Cursor != "" {
		listOpts.Cursor = params.Cursor
	}
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.ApiRoadmapResponse, 0, len(objList.Items))
	for _, roadmap := range objList.Items {
		// Download items from file-manager
		reader, err := r.downloadFile(ctx, roadmap.Spec.Contents)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download roadmap items", err.Error())
		}

		var items []api.ApiRoadmapItem
		err = json.NewDecoder(reader).Decode(&items)
		if err != nil {
			return nil, problems.InternalServerError("Failed to decode roadmap items", err.Error())
		}

		list = append(list, out.MapResponse(roadmap, items))
	}

	return &api.ApiRoadmapListResponse{
		Items: list,
		UnderscoreLinks: api.Links{
			Self: objList.Links.Self,
			Next: objList.Links.Next,
		},
	}, nil
}

// Delete implements ApiRoadmapController.
func (r *RoadmapController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	// Get the roadmap first to retrieve the file ID from Contents field
	ns := id.Environment + "--" + id.Namespace
	roadmap, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	// Delete file from file-manager using the Contents field
	err = file.GetFileManager().DeleteFile(ctx, roadmap.Spec.Contents)
	if err != nil {
		if errors.Is(err, file.ErrNotFound) {
			// File not found is OK, continue to delete CRD
		} else {
			return err
		}
	}

	// Delete CRD
	err = r.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	return nil
}

// Helper methods

// uploadFile uploads the items JSON to file-manager
func (r *RoadmapController) uploadFile(ctx context.Context, itemsMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if len(itemsMarshaled) == 0 {
		return nil, errors.New("items JSON has length 0")
	}

	// Check if hash changed (optimization: skip upload if same)
	localHash, same, err := r.isHashEqual(ctx, id, itemsMarshaled)
	if err != nil {
		return nil, err
	}

	fileId := generateFileId(id)
	fileContentType := "application/json"

	resp := &filesapi.FileUploadResponse{
		FileHash:    localHash,
		FileId:      fileId,
		ContentType: fileContentType,
	}

	if !same {
		resp, err = file.GetFileManager().UploadFile(ctx, fileId, fileContentType, bytes.NewReader(itemsMarshaled))
	}

	return resp, err
}

// isHashEqual checks if the hash of the data matches the stored hash
func (r *RoadmapController) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, error) {
	ns := id.Environment + "--" + id.Namespace
	roadmap, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	hasher := sha256.New()
	hasher.Write(data)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return hash, hash == roadmap.Spec.Hash, nil
}

// downloadFile downloads items JSON from file-manager
func (r *RoadmapController) downloadFile(ctx context.Context, fileId string) (io.Reader, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// generateFileId is defined in apispecification.go and shared across controllers
