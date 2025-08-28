// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"gopkg.in/yaml.v3"

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

	//todo: filesapi does not support delete at the moment
	err = a.Store.Delete(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
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
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	b, err := a.downloadFile(ctx, apiSpec.Spec.Specification)
	if err != nil {
		return res, err
	}

	if b.Len() == 0 {
		return res, errors.New("api specification response is empty")
	}

	m := make(map[string]any)
	err = yaml.Unmarshal(b.Bytes(), &m)
	if err != nil {
		return res, err
	}

	return out.MapResponse(apiSpec, m)
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
	for _, apiSpec := range objList.Items {
		b, err := a.downloadFile(ctx, apiSpec.Spec.Specification)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download resource", err.Error())
		}
		m := make(map[string]any)
		err = yaml.Unmarshal(b.Bytes(), &m)
		if err != nil {
			return nil, problems.InternalServerError("Failed to marshal resource", err.Error())
		}
		resp, err := out.MapResponse(apiSpec, m)
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

	specMarshaled, err := yaml.Marshal(req.Specification)
	if err != nil {
		return res, err
	}
	fileAPIResp, err := a.uploadFile(ctx, specMarshaled, id)
	if err != nil {
		return res, err
	}
	obj, err := in.MapRequest(ctx, specMarshaled, fileAPIResp, id)
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
		if problems.IsNotFound(err) {
			return res, problems.NotFound(resourceId)
		}
		return res, err
	}

	return status.MapResponse(apiSpec.Status.Conditions)
}

func (a *ApiSpecificationController) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if specMarshaled == nil {
		return nil, errors.New("input api specification is nil")
	}

	localHash, same, err := a.isHashEqual(ctx, id, specMarshaled)
	if err != nil {
		return nil, err
	}

	fileId := id.Environment + id.ResourceId + localHash
	fileContentType := "application/yaml"

	resp := &filesapi.FileUploadResponse{
		FileHash:    localHash,
		FileId:      fileId,
		ContentType: fileContentType,
	}

	if !same {
		resp, err = file.GetFileManager().UploadFile(ctx, fileId, fileContentType, bytes.NewReader(specMarshaled))
	}

	return resp, err
}

// isHashEqual checks if the hash of the data is the same as the hash of the api specification in the store.
// will return the hash of the data and a boolean indicating if the hash is the same as in the store
func (a *ApiSpecificationController) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, error) {
	ns := id.Environment + "--" + id.Namespace
	apiSpec, err := a.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	hasher := sha256.New()
	hasher.Write(data)
	hash := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return hash, hash == apiSpec.Spec.Hash, nil
}

func (a *ApiSpecificationController) downloadFile(ctx context.Context, fileId string) (*bytes.Buffer, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}
