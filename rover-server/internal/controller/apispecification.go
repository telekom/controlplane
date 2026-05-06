// Copyright 2025 Deutsche Telekom IT GmbH
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
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
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
	stores *s.Stores
	Store  store.ObjectStore[*roverv1.ApiSpecification]

	// ListApiCategories is a function to list all ApiCategories for validation at upload time.
	// If nil, category validation is skipped.
	ListApiCategories func(ctx context.Context) (*apiv1.ApiCategoryList, error)

	// ErrorMessage is the template message shown when linting fails.
	ErrorMessage string

	// LintTimeout is the HTTP client timeout for external linter calls.
	LintTimeout time.Duration

	// LintAsync controls whether linting runs asynchronously (true) or synchronously (false).
	// When false (default), linting blocks the request so a single store operation includes the result.
	LintAsync bool
}

func NewApiSpecificationController(stores *s.Stores, errorMessage string, lintTimeout time.Duration, lintAsync bool) *ApiSpecificationController {
	ctrl := &ApiSpecificationController{
		stores:       stores,
		Store:        stores.APISpecificationStore,
		ErrorMessage: errorMessage,
		LintTimeout:  lintTimeout,
		LintAsync:    lintAsync,
	}
	if stores.APICategoryStore != nil {
		ctrl.ListApiCategories = func(ctx context.Context) (*apiv1.ApiCategoryList, error) {
			listOpts := store.NewListOpts()
			categoryList, err := stores.APICategoryStore.List(ctx, listOpts)
			if err != nil {
				return nil, err
			}
			result := &apiv1.ApiCategoryList{Items: make([]apiv1.ApiCategory, 0, len(categoryList.Items))}
			for _, item := range categoryList.Items {
				result.Items = append(result.Items, *item)
			}
			return result, nil
		}
	}
	return ctrl
}

// Create implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Create(ctx context.Context, req api.ApiSpecificationCreateRequest) (res api.ApiSpecificationResponse, err error) {
	// Important Hint: This is a declarative API. The client should not create an ApiSpecification, but only use
	// the PUT method. This is similar to how kubernetes works.
	// The main use case for the rover API will be to enable the usage of roverctl
	logr.FromContextOrDiscard(ctx).Info("ApiSpecification: Create not implemented", "request", req)
	return api.ApiSpecificationResponse{},
		fiber.NewError(fiber.StatusNotImplemented, "Create not implemented")
}

// Delete implements server.ApiSpecificationController.
func (a *ApiSpecificationController) Delete(ctx context.Context, resourceId string) error {
	id, err := mapper.ParseResourceId(ctx, resourceId)
	if err != nil {
		return err
	}

	fileId := generateFileId(id)
	err = file.GetFileManager().DeleteFile(ctx, fileId)
	if err != nil {
		if errors.Is(err, file.ErrNotFound) {
			return problems.NotFound(resourceId)
		}
		return err
	}

	ns := id.Environment + "--" + id.Namespace
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

	reader, err := a.downloadFile(ctx, apiSpec.Spec.Specification)
	if err != nil {
		return res, err
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return res, err
	}

	if len(b) == 0 || b == nil {
		return res, errors.New("api specification response is empty")
	}

	m := make(map[string]any)
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return res, err
	}

	return out.MapResponse(ctx, apiSpec, m, a.stores)
}

// GetAll implements server.ApiSpecificationController.
func (a *ApiSpecificationController) GetAll(ctx context.Context, params api.GetAllApiSpecificationsParams) (*api.ApiSpecificationListResponse, error) {
	listOpts := store.NewListOpts()
	listOpts.Cursor = params.Cursor
	store.EnforcePrefix(security.PrefixFromContext(ctx), &listOpts)

	objList, err := a.Store.List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	list := make([]api.ApiSpecificationResponse, 0, len(objList.Items))
	for _, apiSpec := range objList.Items {
		reader, err := a.downloadFile(ctx, apiSpec.Spec.Specification)
		if err != nil {
			return nil, problems.InternalServerError("Failed to download resource", err.Error())
		}

		b, err := io.ReadAll(reader)
		if err != nil {
			return nil, problems.InternalServerError("Failed to read response from reader", err.Error())
		}

		if len(b) == 0 || b == nil {
			return nil, errors.New("api specification response is empty")
		}

		m := make(map[string]any)
		err = yaml.Unmarshal(b, &m)
		if err != nil {
			return nil, problems.InternalServerError("Failed to marshal resource", err.Error())
		}
		resp, err := out.MapResponse(ctx, apiSpec, m, a.stores)
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
		return res, problems.BadRequest(err.Error())
	} else if len(specMarshaled) == 0 {
		return res, problems.BadRequest("spec is empty")
	}

	var apiSpec *roverv1.ApiSpecification
	apiSpec, err = in.ParseSpecification(ctx, string(specMarshaled))
	if err != nil {
		return res, err
	}

	// Fetch the ApiCategory list once for both validation and linting config lookup.
	categoryList := a.fetchApiCategories(ctx)

	// Validate the API category against the known ApiCategories.
	if catErr := a.validateApiCategoryFromList(categoryList, apiSpec.Spec.Category); catErr != nil {
		return res, catErr
	}

	fileAPIResp, err := a.uploadFile(ctx, specMarshaled, id)
	if err != nil {
		return res, err
	}

	err = in.MapRequest(apiSpec, fileAPIResp, id)
	if err != nil {
		return res, problems.BadRequest(err.Error())
	}
	EnsureLabelsOrDie(ctx, apiSpec)

	// Look up the ApiCategory's linting config for this spec's category.
	// If the category has a linter URL, proceed with linting.
	var needsLint bool
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Looking up linting config", "namespace", apiSpec.Namespace, "name", apiSpec.Name,
		"category", apiSpec.Spec.Category, "basepath", apiSpec.Spec.BasePath)
	lintCfg := lintingConfigFromList(categoryList, apiSpec.Spec.Category)
	if lintCfg != nil && lintCfg.URL != "" && lintCfg.Mode != apiv1.LintingModeNone {
		log.V(1).Info("Linting config found, checking whitelists and hash dedup", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		// Fetch existing object for hash dedup comparison.
		existing, _ := a.Store.Get(ctx, apiSpec.Namespace, apiSpec.Name)
		needsLint = a.prepareLinting(lintCfg, apiSpec, existing)
		log.V(1).Info("prepareLinting completed", "namespace", apiSpec.Namespace, "name", apiSpec.Name, "needsLint", needsLint)
	} else {
		log.V(1).Info("No linting config or no URL, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
	}

	// Run linting synchronously (default) so the result is included in the single store write,
	// or dispatch asynchronously if configured.
	if needsLint {
		if a.LintAsync {
			// Store first, then lint in the background and patch afterwards.
			err = a.Store.CreateOrReplace(ctx, apiSpec)
			if err != nil {
				return res, err
			}
			a.dispatchAsyncLint(ctx, apiSpec.Namespace, apiSpec.Name, lintCfg.URL, lintCfg.Ruleset, specMarshaled)
			return a.Get(ctx, resourceId)
		}
		// Synchronous: lint blocks until result is available, then store once.
		if err := a.runSyncLint(ctx, apiSpec, lintCfg.URL, lintCfg.Ruleset, specMarshaled); err != nil {
			// Store the spec with the failed lint result so it's persisted,
			// then return 500 to inform the client about the infrastructure error.
			_ = a.Store.CreateOrReplace(ctx, apiSpec)
			return res, problems.InternalServerError("Linting failed", err.Error())
		}
	}

	err = a.Store.CreateOrReplace(ctx, apiSpec)
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

	return status.MapAPISpecificationResponse(ctx, apiSpec, a.stores)
}

// fetchApiCategories fetches all ApiCategories. Returns nil if the store is not configured.
func (a *ApiSpecificationController) fetchApiCategories(ctx context.Context) *apiv1.ApiCategoryList {
	if a.ListApiCategories == nil {
		return nil
	}
	list, err := a.ListApiCategories(ctx)
	if err != nil {
		logr.FromContextOrDiscard(ctx).Info("Failed to list ApiCategories", "error", err)
		return nil
	}
	return list
}

// validateApiCategoryFromList validates that the given category is a known and active ApiCategory
// using a pre-fetched list. If the list is nil, validation is skipped.
func (a *ApiSpecificationController) validateApiCategoryFromList(categoryList *apiv1.ApiCategoryList, category string) error {
	if categoryList == nil {
		return nil
	}

	found, ok := categoryList.FindByLabelValue(category)
	if !ok {
		allowedLabels := strings.Join(categoryList.AllowedLabelValues(), ", ")
		return problems.BadRequest(
			fmt.Sprintf("ApiCategory %q not found. Allowed values are: [%s]", category, allowedLabels))
	}

	if !found.Spec.Active {
		return problems.BadRequest(
			fmt.Sprintf("ApiCategory %q is not active", category))
	}

	return nil
}

func (a *ApiSpecificationController) uploadFile(ctx context.Context, specMarshaled []byte, id mapper.ResourceIdInfo) (*filesapi.FileUploadResponse, error) {
	if len(specMarshaled) == 0 || specMarshaled == nil {
		return nil, errors.New("input api specification has length 0 or nil")
	}

	localHash, same, err := a.isHashEqual(ctx, id, specMarshaled)
	if err != nil {
		return nil, err
	}

	fileId := generateFileId(id)
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

func (a *ApiSpecificationController) downloadFile(ctx context.Context, fileId string) (io.Reader, error) {
	var b bytes.Buffer
	_, err := file.GetFileManager().DownloadFile(ctx, fileId, &b)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func generateFileId(id mapper.ResourceIdInfo) string {
	fileId := id.Environment + "--" + id.ResourceId //<env>--<group>--<team>--<apiSpecName>
	return fileId
}
