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
	agentin "github.com/telekom/controlplane/rover-server/internal/mapper/agentspecification/in"
	agentout "github.com/telekom/controlplane/rover-server/internal/mapper/agentspecification/out"
	"github.com/telekom/controlplane/rover-server/internal/mapper/status"
	"github.com/telekom/controlplane/rover-server/internal/server"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ server.AgentSpecificationController = &AgentSpecificationControllerImpl{}

type AgentSpecificationControllerImpl struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.AgentSpecification]
}

func NewAgentSpecificationController(stores *s.Stores) *AgentSpecificationControllerImpl {
	return &AgentSpecificationControllerImpl{
		stores: stores,
		Store:  stores.AgentSpecificationStore,
	}
}

func (c *AgentSpecificationControllerImpl) Create(ctx context.Context, req api.AgentSpecificationCreateRequest) (api.AgentSpecificationResponse, error) {
	logr.FromContextOrDiscard(ctx).Info("AgentSpecification: Create not implemented", "request", req)
	return api.AgentSpecificationResponse{}, fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

func (c *AgentSpecificationControllerImpl) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	if cconfig.FeatureFileManager.IsEnabled() {
		agentSpec, getErr := c.Store.Get(ctx, ns, id.Name)
		if getErr != nil && !problems.IsNotFound(getErr) {
			return getErr
		}
		if getErr == nil && agentSpec.Spec.Specification != "" {
			err = file.GetFileManager().DeleteFile(ctx, agentSpec.Spec.Specification)
			if err != nil && !errors.Is(err, file.ErrNotFound) {
				return err
			}
		}
	}

	err = c.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}
	return nil
}

func (c *AgentSpecificationControllerImpl) Get(ctx context.Context, resourceId string) (res api.AgentSpecificationResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	agentSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	specContent, err := c.downloadFile(ctx, agentSpec.Spec.Specification)
	if err != nil {
		return res, err
	}

	return agentout.MapResponse(ctx, agentSpec, specContent, c.stores)
}

func (c *AgentSpecificationControllerImpl) GetAll(ctx context.Context, params api.GetAllAgentSpecificationsParams) (*api.AgentSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := c.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.AgentSpecificationResponse, 0, len(objList.Items))
	for _, agentSpec := range objList.Items {
		specContent, downloadErr := c.downloadFile(ctx, agentSpec.Spec.Specification)
		if downloadErr != nil {
			return nil, downloadErr
		}

		resp, mapErr := agentout.MapResponse(ctx, agentSpec, specContent, c.stores)
		if mapErr != nil {
			return nil, problems.InternalServerError("Failed to map resource", mapErr.Error())
		}
		list = append(list, resp)
	}

	return &api.AgentSpecificationListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

func (c *AgentSpecificationControllerImpl) Update(ctx context.Context, resourceId string, req api.AgentSpecificationUpdateRequest) (res api.AgentSpecificationResponse, err error) {
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

	agentSpec, err := agentin.ParseSpecification(ctx, string(specMarshaled))
	if err != nil {
		return res, err
	}
	if agentSpec.Name != id.Name {
		return res, problems.BadRequest(fmt.Sprintf("agent specification name %q does not match expected name %q", agentSpec.Name, id.Name))
	}

	fileAPIResp, err := c.uploadFile(ctx, specMarshaled, id)
	if err != nil {
		return res, err
	}
	agentin.MapRequest(agentSpec, fileAPIResp, id)

	EnsureLabelsOrDie(ctx, agentSpec)

	err = c.Store.CreateOrReplace(ctx, agentSpec)
	if err != nil {
		return res, err
	}

	return c.Get(ctx, resourceId)
}

func (c *AgentSpecificationControllerImpl) GetStatus(ctx context.Context, resourceId string) (res api.ResourceStatusResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	agentSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapAgentSpecificationResponse(ctx, agentSpec, c.stores)
}

func (c *AgentSpecificationControllerImpl) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil, nil
	}

	if len(specMarshaled) == 0 {
		return nil, errors.New("input specification has length 0")
	}

	localHash, same, existingID, err := c.isHashEqual(ctx, id, specMarshaled)
	if err != nil {
		return nil, err
	}

	fileID, err := resolveFileId(existingID)
	if err != nil {
		return nil, err
	}
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

func (c *AgentSpecificationControllerImpl) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, string, error) {
	ns := id.Environment + "--" + id.Namespace
	agentSpec, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, "", nil
		}
		return "", false, "", err
	}

	hasher := sha256.New()
	hasher.Write(data)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return hash, hash == agentSpec.Spec.Hash, agentSpec.Spec.Specification, nil
}

func (c *AgentSpecificationControllerImpl) downloadFile(ctx context.Context, fileID string) (map[string]any, error) {
	if !cconfig.FeatureFileManager.IsEnabled() {
		return nil, nil
	}

	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileID, &b)
	if err != nil {
		return nil, problems.InternalServerError("Failed to download agent specification", err.Error())
	}

	if b.Len() == 0 {
		return nil, nil
	}

	res := map[string]any{}
	err = yaml.NewDecoder(&b).Decode(&res)
	if err != nil {
		return nil, problems.InternalServerError("Failed to unmarshal agent specification", err.Error())
	}

	return res, nil
}
