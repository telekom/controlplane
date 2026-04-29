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
	"github.com/telekom/controlplane/rover-server/internal/mapper/apichangelog/in"
	"github.com/telekom/controlplane/rover-server/internal/mapper/apichangelog/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ server.ApiChangelogController = &ApiChangelogController{}

type ApiChangelogController struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.ApiChangelog]
}

func NewApiChangelogController(stores *s.Stores) *ApiChangelogController {
	return &ApiChangelogController{
		stores: stores,
		Store:  stores.ApiChangelogStore,
	}
}

// Create implements ApiChangelogController.
func (c *ApiChangelogController) Create(ctx context.Context, req api.ApiChangelogCreateRequest) (api.ApiChangelogResponse, error) {
	log.Infof("ApiChangelog: Create not implemented. ApiChangelog is: %+v", req)
	return api.ApiChangelogResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Update implements ApiChangelogController.
func (c *ApiChangelogController) Update(ctx context.Context, resourceId string, req api.ApiChangelogUpdateRequest) (api.ApiChangelogResponse, error) {
	var res api.ApiChangelogResponse

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

	expectedName := in.MakeChangelogName(req.BasePath)
	if expectedName != id.Name {
		return res, problems.BadRequest("basePath " + req.BasePath + " does not match resource ID " + resourceId)
	}

	return c.createOrUpdateChangelog(ctx, id, req.BasePath, req.Items)
}

func (c *ApiChangelogController) createOrUpdateChangelog(ctx context.Context, id mapper.ResourceIdInfo, basePath string, items []api.ApiChangelogItem) (api.ApiChangelogResponse, error) {
	var res api.ApiChangelogResponse

	itemsMarshaled, err := json.Marshal(items)
	if err != nil {
		return res, problems.BadRequest("failed to marshal items: " + err.Error())
	}

	fileAPIResp, err := c.uploadFile(ctx, itemsMarshaled, id)
	if err != nil {
		return res, err
	}

	changelog, err := in.MapRequest(basePath, fileAPIResp, id)
	if err != nil {
		return res, err
	}
	EnsureLabelsOrDie(ctx, changelog)

	err = c.Store.CreateOrReplace(ctx, changelog)
	if err != nil {
		return res, err
	}

	return out.MapResponse(changelog, items), nil
}

// Get implements ApiChangelogController.
func (c *ApiChangelogController) Get(ctx context.Context, resourceId string) (api.ApiChangelogResponse, error) {
	var res api.ApiChangelogResponse

	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	// Download items from file-manager
	reader, err := c.downloadFile(ctx, changelog.Spec.Contents)
	if err != nil {
		return res, errors.Wrap(err, "failed to download changelog items from file-manager")
	}

	var items []api.ApiChangelogItem
	err = json.NewDecoder(reader).Decode(&items)
	if err != nil {
		return res, errors.Wrap(err, "failed to decode changelog items")
	}

	return out.MapResponse(changelog, items), nil
}

// GetAll implements ApiChangelogController.
func (c *ApiChangelogController) GetAll(ctx context.Context, params api.GetAllApiChangelogsParams) (*api.ApiChangelogListResponse, error) {
	listOpts := store.NewListOpts()
	if params.Cursor != "" {
		listOpts.Cursor = params.Cursor
	}
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := c.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.ApiChangelogResponse, 0, len(objList.Items))
	for _, changelog := range objList.Items {
		// Download items from file-manager
		reader, err := c.downloadFile(ctx, changelog.Spec.Contents)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download changelog items", err.Error())
		}

		var items []api.ApiChangelogItem
		err = json.NewDecoder(reader).Decode(&items)
		if err != nil {
			return nil, problems.InternalServerError("Failed to decode changelog items", err.Error())
		}

		list = append(list, out.MapResponse(changelog, items))
	}

	return &api.ApiChangelogListResponse{
		Items: list,
		UnderscoreLinks: api.Links{
			Self: objList.Links.Self,
			Next: objList.Links.Next,
		},
	}, nil
}

// Delete implements ApiChangelogController.
func (c *ApiChangelogController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	// Get the changelog first to retrieve the file ID from Contents field
	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	// Delete file from file-manager using the Contents field
	err = file.GetFileManager().DeleteFile(ctx, changelog.Spec.Contents)
	if err != nil {
		if errors.Is(err, file.ErrNotFound) {
			// File not found is OK, continue to delete CRD
		} else {
			return err
		}
	}

	// Delete CRD
	err = c.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	return nil
}

// GetStatus implements ApiChangelogController.
func (c *ApiChangelogController) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapResponse(ctx, changelog)
}

// Helper methods

// uploadFile uploads the items JSON to file-manager
func (c *ApiChangelogController) uploadFile(ctx context.Context, itemsMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if len(itemsMarshaled) == 0 {
		return nil, errors.New("items JSON has length 0")
	}

	// Check if hash changed (optimization: skip upload if same)
	localHash, same, err := c.isHashEqual(ctx, id, itemsMarshaled)
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
func (c *ApiChangelogController) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, error) {
	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	hasher := sha256.New()
	hasher.Write(data)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return hash, hash == changelog.Spec.Hash, nil
}

// downloadFile downloads items JSON from file-manager
func (c *ApiChangelogController) downloadFile(ctx context.Context, fileId string) (io.Reader, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// generateFileId is defined in apispecification.go and shared across controllers
