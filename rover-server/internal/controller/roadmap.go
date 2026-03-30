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

// Create is not implemented - use PUT (Update) for declarative resource management
// This follows the Kubernetes pattern where clients should use PUT to create/update resources
func (r *RoadmapController) Create(ctx context.Context, req api.RoadmapRequest) (res api.RoadmapResponse, err error) {
	return api.RoadmapResponse{}, fiber.NewError(fiber.StatusNotImplemented, "Create not implemented. Use PUT /roadmaps/{resourceId} instead")
}

// Update updates an existing roadmap
func (r *RoadmapController) Update(ctx context.Context, resourceId string, req api.RoadmapRequest) (res api.RoadmapResponse, err error) {
	// Validate request
	if req.ResourceName == "" {
		return res, problems.BadRequest("resourceName must not be empty")
	}
	if req.ResourceType != api.RoadmapResourceTypeAPI && req.ResourceType != api.RoadmapResourceTypeEvent {
		return res, problems.BadRequest("resourceType must be either 'API' or 'Event'")
	}
	if len(req.Items) == 0 {
		return res, problems.BadRequest("items array must contain at least one item")
	}

	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return res, err
	}

	// Marshal items to JSON
	itemsMarshaled, err := json.Marshal(req.Items)
	if err != nil {
		return res, problems.BadRequest("failed to marshal items: " + err.Error())
	}

	// Upload to file-manager
	fileAPIResp, err := r.uploadFile(ctx, itemsMarshaled, id)
	if err != nil {
		return res, err
	}

	// Create/Update CRD (declarative PUT - creates if not exists, updates if exists)
	ns := id.Environment + "--" + id.Namespace
	roadmap := &roverv1.Roadmap{}
	roadmap.TypeMeta = metav1.TypeMeta{
		Kind:       "Roadmap",
		APIVersion: "rover.cp.ei.telekom.de/v1",
	}
	roadmap.Name = id.Name
	roadmap.Namespace = ns
	roadmap.Spec.ResourceName = req.ResourceName
	roadmap.Spec.ResourceType = roverv1.ResourceType(req.ResourceType)
	roadmap.Spec.Roadmap = fileAPIResp.FileId
	roadmap.Spec.Hash = fileAPIResp.FileHash
	EnsureLabelsOrDie(ctx, roadmap)

	err = r.Store.CreateOrReplace(ctx, roadmap)
	if err != nil {
		return res, err
	}

	return r.Get(ctx, resourceId)
}

// Get retrieves a roadmap by ID
func (r *RoadmapController) Get(ctx context.Context, resourceId string) (res api.RoadmapResponse, err error) {
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
	reader, err := r.downloadFile(ctx, roadmap.Spec.Roadmap)
	if err != nil {
		return res, errors.Wrap(err, "failed to download roadmap items from file-manager")
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return res, errors.Wrap(err, "failed to read downloaded roadmap items")
	}

	if len(b) == 0 {
		return res, errors.Errorf("roadmap items response is empty (fileId: %s)", roadmap.Spec.Roadmap)
	}

	var items []api.RoadmapItem
	err = json.Unmarshal(b, &items)
	if err != nil {
		preview := string(b)
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		return res, errors.Wrapf(err, "failed to unmarshal roadmap items (got %d bytes): %s", len(b), preview)
	}

	// Map to response
	return api.RoadmapResponse{
		Id:           id.ResourceId,
		Name:         roadmap.Name,
		ResourceName: roadmap.Spec.ResourceName,
		ResourceType: api.RoadmapResponseResourceType(roadmap.Spec.ResourceType),
		Items:        items,
		Status:       *mapStatus(roadmap),
	}, nil
}

// GetAll lists all roadmaps, optionally filtered by resourceType
func (r *RoadmapController) GetAll(ctx context.Context, params api.GetAllRoadmapsParams) (*api.RoadmapListResponse, error) {
	listOpts := store.NewListOpts()
	if params.Cursor != "" {
		listOpts.Cursor = params.Cursor
	}
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.RoadmapResponse, 0, len(objList.Items))
	for _, roadmap := range objList.Items {
		// Apply resourceType filter if specified
		if params.ResourceType != "" && roadmap.Spec.ResourceType != roverv1.ResourceType(params.ResourceType) {
			continue
		}

		// Download items from file-manager
		reader, err := r.downloadFile(ctx, roadmap.Spec.Roadmap)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download roadmap items", err.Error())
		}

		b, err := io.ReadAll(reader)
		if err != nil {
			return nil, problems.InternalServerError("Failed to read roadmap items", err.Error())
		}

		if len(b) == 0 {
			return nil, errors.New("roadmap items response is empty")
		}

		var items []api.RoadmapItem
		err = json.Unmarshal(b, &items)
		if err != nil {
			return nil, problems.InternalServerError("Failed to unmarshal roadmap items", err.Error())
		}

		// Create resource ID
		resourceId := roadmap.Namespace[len(security.PrefixFromContext(ctx)):] + "--" + roadmap.Name
		if len(roadmap.Namespace) > 0 && roadmap.Namespace[0:len(security.PrefixFromContext(ctx))] != security.PrefixFromContext(ctx) {
			// Namespace doesn't match prefix, use full namespace
			resourceId = roadmap.Namespace + "--" + roadmap.Name
		}

		resp := api.RoadmapResponse{
			Id:           resourceId,
			Name:         roadmap.Name,
			ResourceName: roadmap.Spec.ResourceName,
			ResourceType: api.RoadmapResponseResourceType(roadmap.Spec.ResourceType),
			Items:        items,
			Status:       *mapStatus(roadmap),
		}
		list = append(list, resp)
	}

	return &api.RoadmapListResponse{
		Items: list,
		UnderscoreLinks: api.Links{
			Self: objList.Links.Self,
			Next: objList.Links.Next,
		},
	}, nil
}

// Delete deletes a roadmap
func (r *RoadmapController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	// Delete file from file-manager
	fileId := generateFileId(id)
	err = file.GetFileManager().DeleteFile(ctx, fileId)
	if err != nil {
		if errors.Is(err, file.ErrNotFound) {
			// File not found is OK, continue to delete CRD
		} else {
			return err
		}
	}

	// Delete CRD
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

// removeDuplicates removes existing roadmaps with the same resourceName + resourceType
// This implements the duplicate removal logic from the Java system
func (r *RoadmapController) removeDuplicates(ctx context.Context, resourceName string, resourceType roverv1.ResourceType, environment string) error {
	// List all roadmaps in the environment/namespace
	listOpts := store.NewListOpts()
	store.EnforcePrefix(environment+"--", &listOpts)

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return err
	}

	// Find and delete roadmaps with matching resourceName + resourceType
	for _, existing := range objList.Items {
		if existing.Spec.ResourceName == resourceName && existing.Spec.ResourceType == resourceType {
			// Delete file from file-manager
			fileId := existing.Spec.Roadmap
			_ = file.GetFileManager().DeleteFile(ctx, fileId) // Ignore errors

			// Delete CRD
			err = r.Store.Delete(ctx, existing.Namespace, existing.Name)
			if err != nil && !problems.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// mapStatus maps CRD status to response status
func mapStatus(roadmap *roverv1.Roadmap) *api.RoadmapStatus {
	if roadmap.Status.Conditions == nil || len(roadmap.Status.Conditions) == 0 {
		return nil
	}

	status := &api.RoadmapStatus{}
	for _, cond := range roadmap.Status.Conditions {
		if cond.Type == "Ready" {
			status.Ready = cond.Status == "True"
			status.Message = cond.Message
		}
		if cond.Type == "Processing" {
			status.Processing = cond.Status == "True"
		}
	}

	return status
}

// normalizeResourceName normalizes the resource name for use as a Kubernetes resource name
func normalizeResourceName(resourceName string) string {
	// Replace slashes with hyphens and convert to lowercase
	// This is a simplified version - a real implementation would need more robust normalization
	name := resourceName
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	return name
}
