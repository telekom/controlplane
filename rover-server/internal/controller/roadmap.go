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
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	statusmapper "github.com/telekom/controlplane/rover-server/internal/mapper/status"
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

// CreateApiRoadmap implements the ServerInterface for creating roadmaps (POST)
// Accepts the old Java API format with basePath
func (r *RoadmapController) CreateApiRoadmap(c *fiber.Ctx) error {
	ctx := c.UserContext()

	var req api.ApiRoadmapCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return problems.BadRequest("Failed to parse request body: " + err.Error())
	}

	// Validate request
	if req.BasePath == "" {
		return problems.BadRequest("basePath must not be empty")
	}
	if len(req.Items) == 0 {
		return problems.BadRequest("items array must contain at least one item")
	}

	// Call internal logic to create the roadmap
	response, err := r.createOrUpdateRoadmap(ctx, req.BasePath, req.Items)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// UpdateApiRoadmap implements the ServerInterface for updating roadmaps (PUT)
// Accepts the old Java API format with basePath
func (r *RoadmapController) UpdateApiRoadmap(c *fiber.Ctx, apiRoadmapId string) error {
	ctx := c.UserContext()

	var req api.ApiRoadmapUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return problems.BadRequest("Failed to parse request body: " + err.Error())
	}

	// Validate request
	if req.BasePath == "" {
		return problems.BadRequest("basePath must not be empty")
	}
	if len(req.Items) == 0 {
		return problems.BadRequest("items array must contain at least one item")
	}

	// Call internal logic to update the roadmap
	response, err := r.createOrUpdateRoadmap(ctx, req.BasePath, req.Items)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// createOrUpdateRoadmap is the internal shared logic for creating/updating roadmaps
// It transforms the old API format (basePath) to the new CRD structure (TypedObjectRef)
func (r *RoadmapController) createOrUpdateRoadmap(ctx context.Context, basePath string, items []api.ApiRoadmapItem) (api.ApiRoadmapResponse, error) {
	var res api.ApiRoadmapResponse

	// Get security context for namespace construction
	secCtx, ok := security.FromContext(ctx)
	if !ok {
		return res, problems.InternalServerError("Invalid Context", "Security context not found")
	}

	// Generate roadmap name using the new pattern: api--<specialName>
	roadmapName := makeRoadmapName(basePath)

	// Construct namespace from environment and team
	// For now, extract from security context - we need the team from the token
	// The namespace format is: <environment>--<group>--<team>
	namespace := secCtx.Environment + "--" + secCtx.Group + "--" + secCtx.Team

	// Marshal items to JSON
	itemsMarshaled, err := json.Marshal(items)
	if err != nil {
		return res, problems.BadRequest("failed to marshal items: " + err.Error())
	}

	// Create a temporary ResourceIdInfo for file operations
	// We use the roadmap name as the resource identifier
	tempId := mapper.ResourceIdInfo{
		ResourceId:  secCtx.Group + "--" + secCtx.Team + "--" + roadmapName,
		Environment: secCtx.Environment,
		Namespace:   secCtx.Group + "--" + secCtx.Team,
		Name:        roadmapName,
	}

	// Upload to file-manager
	fileAPIResp, err := r.uploadFile(ctx, itemsMarshaled, tempId)
	if err != nil {
		return res, err
	}

	// Construct TypedObjectRef to specification resource
	// The ApiSpecification name is derived from basePath (e.g., "/eni/my-api/v1" -> "eni-my-api-v1")
	apiSpecName := makeApiSpecificationName(basePath)
	apiSpecRef := types.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ApiSpecification",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectRef: types.ObjectRef{
			Name:      apiSpecName,
			Namespace: namespace,
		},
	}

	// Create/Update Roadmap CRD
	roadmap := &roverv1.Roadmap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Roadmap",
			APIVersion: "rover.cp.ei.telekom.de/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roadmapName,
			Namespace: namespace,
			Annotations: map[string]string{
				"rover.cp.ei.telekom.de/basePath": basePath,
			},
		},
		Spec: roverv1.RoadmapSpec{
			SpecificationRef: apiSpecRef,
			Contents:         fileAPIResp.FileId,
			Hash:             fileAPIResp.FileHash,
		},
	}
	EnsureLabelsOrDie(ctx, roadmap)

	err = r.Store.CreateOrReplace(ctx, roadmap)
	if err != nil {
		return res, err
	}

	// Remove any duplicate roadmaps pointing to the same specification
	if err := r.removeDuplicates(ctx, roadmap); err != nil {
		log.Warnf("Failed to remove duplicate roadmaps for basePath %s: %v", basePath, err)
	}

	// Download items to return in response
	reader, err := r.downloadFile(ctx, fileAPIResp.FileId)
	if err != nil {
		return res, errors.Wrap(err, "failed to download roadmap items from file-manager")
	}

	var responseItems []api.ApiRoadmapItem
	err = json.NewDecoder(reader).Decode(&responseItems)
	if err != nil {
		return res, errors.Wrap(err, "failed to decode roadmap items")
	}

	// Construct response
	resourceId := secCtx.Group + "--" + secCtx.Team + "--" + roadmapName
	return api.ApiRoadmapResponse{
		BasePath: basePath,
		Id:       resourceId,
		Name:     roadmapName,
		Items:    responseItems,
		Status:   statusmapper.MapStatus(roadmap.GetConditions(), roadmap.GetGeneration()),
	}, nil
}

// versionSuffixRe matches a major version suffix like "-v1", "-v2", "-v10"
var versionSuffixRe = regexp.MustCompile(`-v\d+$`)

// makeRoadmapName generates the roadmap name from an API basePath.
// The name is the normalized basePath with the major version suffix removed.
// Example: "/eni/my-api/v1" → "eni-my-api"
// Note: the name must NOT contain "--" since that is used as a separator in resource IDs.
func makeRoadmapName(basePath string) string {
	normalized := labelutil.NormalizeValue(basePath)
	specialName := versionSuffixRe.ReplaceAllString(normalized, "")
	return labelutil.NormalizeNameValue(specialName)
}

// makeApiSpecificationName generates the ApiSpecification name from basePath
// This matches the logic in ApiSpecification's MakeName() function
func makeApiSpecificationName(basePath string) string {
	return labelutil.NormalizeValue(basePath)
}

// GetApiRoadmap retrieves a roadmap by ID
func (r *RoadmapController) GetApiRoadmap(c *fiber.Ctx, apiRoadmapId string) error {
	ctx := c.UserContext()

	// Parse the apiRoadmapId to extract namespace and name
	// The ID format is: group--team--roadmapName (e.g., "eni--hyperion--api--my-api")
	id, err := mapper.ParseResourceId(ctx, apiRoadmapId)
	if err != nil {
		return err
	}

	ns := id.Environment + "--" + id.Namespace
	roadmap, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(apiRoadmapId)
		}
		return err
	}

	// Download items from file-manager
	reader, err := r.downloadFile(ctx, roadmap.Spec.Contents)
	if err != nil {
		return errors.Wrap(err, "failed to download roadmap items from file-manager")
	}

	var items []api.ApiRoadmapItem
	err = json.NewDecoder(reader).Decode(&items)
	if err != nil {
		return errors.Wrap(err, "failed to decode roadmap items")
	}

	// Extract basePath from roadmap annotations
	basePath := ""
	if roadmap.Annotations != nil {
		basePath = roadmap.Annotations["rover.cp.ei.telekom.de/basePath"]
	}
	if basePath == "" {
		// Fallback: try to derive from specification name
		basePath = "/" + strings.ReplaceAll(roadmap.Spec.SpecificationRef.Name, "-", "/")
	}

	// Construct response
	response := api.ApiRoadmapResponse{
		BasePath: basePath,
		Id:       id.ResourceId,
		Name:     roadmap.Name,
		Items:    items,
		Status:   statusmapper.MapStatus(roadmap.GetConditions(), roadmap.GetGeneration()),
	}

	return c.JSON(response)
}

// GetAll lists all roadmaps, optionally filtered by resourceType
// GetAllApiRoadmaps lists all API roadmaps
func (r *RoadmapController) GetAllApiRoadmaps(c *fiber.Ctx, params api.GetAllApiRoadmapsParams) error {
	ctx := c.UserContext()

	listOpts := store.NewListOpts()
	if params.Cursor != "" {
		listOpts.Cursor = params.Cursor
	}
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return err
	}

	list := make([]api.ApiRoadmapResponse, 0, len(objList.Items))
	for _, roadmap := range objList.Items {
		// Download items from file-manager
		reader, err := r.downloadFile(ctx, roadmap.Spec.Contents)
		if err != nil {
			return problems.InternalServerError("Failed to download roadmap items", err.Error())
		}

		var items []api.ApiRoadmapItem
		err = json.NewDecoder(reader).Decode(&items)
		if err != nil {
			return problems.InternalServerError("Failed to decode roadmap items", err.Error())
		}

		// Extract basePath from annotations
		basePath := ""
		if roadmap.Annotations != nil {
			basePath = roadmap.Annotations["rover.cp.ei.telekom.de/basePath"]
		}
		if basePath == "" {
			basePath = "/" + strings.ReplaceAll(roadmap.Spec.SpecificationRef.Name, "-", "/")
		}

		resourceId := mapper.MakeResourceId(roadmap)
		resp := api.ApiRoadmapResponse{
			BasePath: basePath,
			Id:       resourceId,
			Name:     roadmap.Name,
			Items:    items,
			Status:   statusmapper.MapStatus(roadmap.GetConditions(), roadmap.GetGeneration()),
		}
		list = append(list, resp)
	}

	response := api.ApiRoadmapListResponse{
		Items: list,
		UnderscoreLinks: api.Links{
			Self: objList.Links.Self,
			Next: objList.Links.Next,
		},
	}

	return c.JSON(response)
}

// DeleteApiRoadmap deletes a roadmap by ID
func (r *RoadmapController) DeleteApiRoadmap(c *fiber.Ctx, apiRoadmapId string) error {
	ctx := c.UserContext()

	// Parse the apiRoadmapId to extract namespace and name
	id, err := mapper.ParseResourceId(ctx, apiRoadmapId)
	if err != nil {
		return err
	}

	// Get the roadmap first to retrieve the file ID from Contents field
	ns := id.Environment + "--" + id.Namespace
	roadmap, err := r.Store.Get(ctx, ns, id.Name)
	if err != nil {
		if problems.IsNotFound(err) {
			return problems.NotFound(apiRoadmapId)
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
			return problems.NotFound(apiRoadmapId)
		}
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
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

// removeDuplicates removes existing roadmaps pointing to the same specification
// This implements the duplicate removal logic from the Java system
// It excludes the newly created roadmap (by name) to avoid deleting what we just created
func (r *RoadmapController) removeDuplicates(ctx context.Context, newRoadmap *roverv1.Roadmap) error {
	// List all roadmaps in the same namespace
	listOpts := store.NewListOpts()
	store.EnforcePrefix(newRoadmap.Namespace+"/", &listOpts)

	objList, err := r.Store.List(ctx, listOpts)
	if err != nil {
		return err
	}

	// Find and delete roadmaps pointing to the same specification (excluding the one we just created)
	for _, existing := range objList.Items {
		if existing.Name == newRoadmap.Name {
			// Skip the roadmap we just created
			continue
		}
		// Compare specification references (Name and Namespace)
		if existing.Spec.SpecificationRef.Name == newRoadmap.Spec.SpecificationRef.Name &&
			existing.Spec.SpecificationRef.Namespace == newRoadmap.Spec.SpecificationRef.Namespace {
			// Delete file from file-manager
			fileId := existing.Spec.Contents
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

// generateFileId is defined in apispecification.go and shared across controllers
