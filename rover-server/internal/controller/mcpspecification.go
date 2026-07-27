// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	mcpin "github.com/telekom/controlplane/rover-server/internal/mapper/mcpspecification/in"
	mcpout "github.com/telekom/controlplane/rover-server/internal/mapper/mcpspecification/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ server.McpSpecificationController = &McpSpecificationControllerImpl{}

type McpSpecificationControllerImpl struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.McpSpecification]
}

func NewMcpSpecificationController(stores *s.Stores) *McpSpecificationControllerImpl {
	return &McpSpecificationControllerImpl{
		stores: stores,
		Store:  stores.McpSpecificationStore,
	}
}

func (c *McpSpecificationControllerImpl) Create(ctx context.Context, req api.McpSpecificationCreateRequest) (api.McpSpecificationResponse, error) {
	logr.FromContextOrDiscard(ctx).Info("McpSpecification: Create not implemented", "request", req)
	return api.McpSpecificationResponse{}, fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

func (c *McpSpecificationControllerImpl) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	if cconfig.FeatureFileManager.IsEnabled() {
		fileID := generateMcpFileID(id)
		err = file.GetFileManager().DeleteFile(ctx, fileID)
		if err != nil && !errors.Is(err, file.ErrNotFound) {
			return err
		}
	}

	ns := id.Environment + "--" + id.Namespace
	err = c.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}
	return nil
}

func (c *McpSpecificationControllerImpl) Get(ctx context.Context, resourceId string) (res api.McpSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	mcpSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	var specContent map[string]any
	if cconfig.FeatureFileManager.IsEnabled() {
		reader, downloadErr := c.downloadFile(ctx, mcpSpec.Spec.Specification)
		if downloadErr != nil {
			return res, downloadErr
		}

		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			return res, readErr
		}
		if len(data) > 0 {
			if unmarshalErr := yaml.Unmarshal(data, &specContent); unmarshalErr != nil {
				return res, unmarshalErr
			}
		}
	}

	return mcpout.MapResponse(ctx, mcpSpec, specContent, c.stores)
}

func (c *McpSpecificationControllerImpl) GetAll(ctx context.Context, params api.GetAllMcpSpecificationsParams) (*api.McpSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := c.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.McpSpecificationResponse, 0, len(objList.Items))
	for _, mcpSpec := range objList.Items {
		var specContent map[string]any
		if cconfig.FeatureFileManager.IsEnabled() {
			reader, downloadErr := c.downloadFile(ctx, mcpSpec.Spec.Specification)
			if downloadErr != nil {
				return nil, problems.InternalServerError("Failed to download resource", downloadErr.Error())
			}

			data, readErr := io.ReadAll(reader)
			if readErr != nil {
				return nil, problems.InternalServerError("Failed to read response", readErr.Error())
			}
			if len(data) > 0 {
				if unmarshalErr := yaml.Unmarshal(data, &specContent); unmarshalErr != nil {
					return nil, problems.InternalServerError("Failed to unmarshal resource", unmarshalErr.Error())
				}
			}
		}

		resp, mapErr := mcpout.MapResponse(ctx, mcpSpec, specContent, c.stores)
		if mapErr != nil {
			return nil, problems.InternalServerError("Failed to map resource", mapErr.Error())
		}
		list = append(list, resp)
	}

	return &api.McpSpecificationListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

func (c *McpSpecificationControllerImpl) Update(ctx context.Context, resourceId string, req api.McpSpecificationUpdateRequest) (res api.McpSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	specMarshaled, err := yaml.Marshal(req.Specification)
	if err != nil {
		return res, problems.BadRequest(err.Error())
	}
	if len(specMarshaled) == 0 {
		return res, problems.BadRequest("spec is empty")
	}

	mcpSpec, err := mcpin.ParseSpecification(ctx, string(specMarshaled))
	if err != nil {
		return res, err
	}
	if mcpSpec.Name != id.Name {
		return res, problems.BadRequest(fmt.Sprintf("mcp specification name %q does not match expected name %q", mcpSpec.Name, id.Name))
	}

	if cconfig.FeatureFileManager.IsEnabled() {
		fileAPIResp, uploadErr := c.uploadFile(ctx, specMarshaled, id)
		if uploadErr != nil {
			return res, uploadErr
		}
		mcpin.MapRequest(mcpSpec, fileAPIResp, id)
	} else {
		mcpin.MapRequestWithoutFile(mcpSpec, id)
	}

	EnsureLabelsOrDie(ctx, mcpSpec)

	err = c.Store.CreateOrReplace(ctx, mcpSpec)
	if err != nil {
		return res, err
	}

	return c.Get(ctx, resourceId)
}

func (c *McpSpecificationControllerImpl) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	mcpSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapMcpSpecificationResponse(ctx, mcpSpec, c.stores)
}

func (c *McpSpecificationControllerImpl) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if len(specMarshaled) == 0 {
		return nil, errors.New("input specification has length 0")
	}

	localHash, same, err := c.isHashEqual(ctx, id, specMarshaled)
	if err != nil {
		return nil, err
	}

	fileID := generateMcpFileID(id)
	fileContentType := "application/yaml"

	resp := &filesapi.FileUploadResponse{
		FileHash:    localHash,
		FileId:      fileID,
		ContentType: fileContentType,
	}

	if !same {
		resp, err = file.GetFileManager().UploadFile(ctx, fileID, fileContentType, bytes.NewReader(specMarshaled))
	}

	return resp, err
}

func (c *McpSpecificationControllerImpl) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, error) {
	ns := id.Environment + "--" + id.Namespace
	mcpSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	hasher := sha256.New()
	hasher.Write(data)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return hash, hash == mcpSpec.Spec.Hash, nil
}

func (c *McpSpecificationControllerImpl) downloadFile(ctx context.Context, fileID string) (io.Reader, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileID, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func generateMcpFileID(id mapper.ResourceIdInfo) string {
	return id.Environment + "--" + id.ResourceId
}
