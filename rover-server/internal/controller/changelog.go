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
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	s "github.com/telekom/controlplane/rover-server/pkg/store"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ChangelogController struct {
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.Changelog]
}

func NewChangelogController(stores *s.Stores) *ChangelogController {
	return &ChangelogController{
		stores: stores,
		Store:  stores.ChangelogStore,
	}
}

func (c *ChangelogController) Create(ctx context.Context, req api.ChangelogRequest) (res api.ChangelogResponse, err error) {
	return api.ChangelogResponse{}, fiber.NewError(fiber.StatusNotImplemented, "Create not implemented. Use PUT /changelogs/{resourceId} instead")
}

func (c *ChangelogController) Get(ctx context.Context, resourceId string) (res api.ChangelogResponse, err error) {
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

	reader, err := c.downloadFile(ctx, changelog.Spec.Changelog)
	if err != nil {
		return res, err
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return res, err
	}

	if len(b) == 0 {
		return res, errors.New("changelog response is empty")
	}

	var items []api.ChangelogItem
	if err = json.Unmarshal(b, &items); err != nil {
		return res, err
	}

	return api.ChangelogResponse{
		Id:           resourceId,
		Name:         changelog.Name,
		ResourceName: changelog.Spec.ResourceName,
		ResourceType: api.ChangelogResponseResourceType(changelog.Spec.ResourceType),
		Items:        items,
	}, nil
}

func (c *ChangelogController) GetAll(ctx context.Context, params api.GetAllChangelogsParams) (*api.ChangelogListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := c.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.ChangelogResponse, 0, len(objList.Items))
	for _, changelog := range objList.Items {
		if params.ResourceType != "" &&
			string(changelog.Spec.ResourceType) != string(params.ResourceType) {
			continue
		}

		reader, err := c.downloadFile(ctx, changelog.Spec.Changelog)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download resource", err.Error())
		}

		b, err := io.ReadAll(reader)
		if err != nil {
			return nil, problems.InternalServerError("Failed to read resource", err.Error())
		}

		var items []api.ChangelogItem
		if err = json.Unmarshal(b, &items); err != nil {
			return nil, problems.InternalServerError("Failed to parse resource", err.Error())
		}

		resourceId := changelog.Namespace + "--" + changelog.Name
		list = append(list, api.ChangelogResponse{
			Id:           resourceId,
			Name:         changelog.Name,
			ResourceName: changelog.Spec.ResourceName,
			ResourceType: api.ChangelogResponseResourceType(changelog.Spec.ResourceType),
			Items:        items,
		})
	}

	return &api.ChangelogListResponse{
		UnderscoreLinks: api.Links{
			Next: objList.Links.Next,
			Self: objList.Links.Self,
		},
		Items: list,
	}, nil
}

func (c *ChangelogController) Update(ctx context.Context, resourceId string, req api.ChangelogRequest) (res api.ChangelogResponse, err error) {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	if err = validateChangelogRequest(req); err != nil {
		return res, err
	}

	itemsMarshaled, err := json.Marshal(req.Items)
	if err != nil {
		return res, problems.BadRequest("failed to marshal items: " + err.Error())
	}

	fileAPIResp, err := c.uploadFile(ctx, itemsMarshaled, id)
	if err != nil {
		return res, err
	}

	ns := id.Environment + "--" + id.Namespace
	changelog := &roverv1.Changelog{}
	changelog.TypeMeta = metav1.TypeMeta{
		Kind:       "Changelog",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	changelog.Name = id.Name
	changelog.Namespace = ns
	changelog.Spec.ResourceName = req.ResourceName
	changelog.Spec.ResourceType = roverv1.ResourceType(req.ResourceType)
	changelog.Spec.Changelog = fileAPIResp.FileId
	changelog.Spec.Hash = fileAPIResp.FileHash
	EnsureLabelsOrDie(ctx, changelog)

	err = c.Store.CreateOrReplace(ctx, changelog)
	if err != nil {
		return res, err
	}

	return c.Get(ctx, resourceId)
}

func (c *ChangelogController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	fileId := changelog.Spec.Changelog
	err = file.GetFileManager().DeleteFile(ctx, fileId)
	if err != nil && !errors.Is(err, file.ErrNotFound) {
		return err
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

func (c *ChangelogController) uploadFile(ctx context.Context, itemsMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if len(itemsMarshaled) == 0 {
		return nil, errors.New("input changelog has length 0")
	}

	localHash := computeHash(itemsMarshaled)
	_, same, err := c.isHashEqual(ctx, id, itemsMarshaled)
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

func (c *ChangelogController) isHashEqual(ctx context.Context, id mapper.ResourceIdInfo, data []byte) (string, bool, error) {
	ns := id.Environment + "--" + id.Namespace
	changelog, err := c.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	localHash := computeHash(data)
	return localHash, localHash == changelog.Spec.Hash, nil
}

func (c *ChangelogController) downloadFile(ctx context.Context, fileId string) (io.Reader, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func computeHash(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

var semverRegex = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func validateChangelogRequest(req api.ChangelogRequest) error {
	if req.ResourceName == "" {
		return problems.BadRequest("resourceName must not be empty")
	}

	if req.ResourceType != api.ChangelogResourceTypeAPI && req.ResourceType != api.ChangelogResourceTypeEvent {
		return problems.BadRequest("resourceType must be either 'API' or 'Event'")
	}

	if len(req.Items) == 0 {
		return problems.BadRequest("items array must contain at least one item")
	}

	for i, item := range req.Items {
		dateStr := item.Date.Format("2006-01-02")
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			return problems.BadRequest(fmt.Sprintf("item[%d]: date must match format yyyy-MM-dd", i))
		}

		if item.Version == "" {
			return problems.BadRequest(fmt.Sprintf("item[%d]: version is required", i))
		}

		if !semverRegex.MatchString(item.Version) {
			return problems.BadRequest(fmt.Sprintf("item[%d]: version must match semantic versioning format (e.g., 1.2.3)", i))
		}

		if item.Description == "" {
			return problems.BadRequest(fmt.Sprintf("item[%d]: description is required", i))
		}
	}

	return nil
}
