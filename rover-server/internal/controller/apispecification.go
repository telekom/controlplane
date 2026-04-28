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

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/pkg/errors"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
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
	stores    *s.Stores
	Store     store.ObjectStore[*roverv1.ApiSpecification]
	ZoneStore store.ObjectStore[*adminv1.Zone]
	Linter    oaslint.Linter

	// ListApiCategories is a function to list all ApiCategories for validation at upload time.
	// If nil, category validation is skipped.
	ListApiCategories func(ctx context.Context) (*apiv1.ApiCategoryList, error)

	// Whitelists and error message for linting, from config.
	WhitelistedBasepaths  map[string]struct{}
	WhitelistedCategories map[string]struct{}
	ErrorMessage          string
}

func NewApiSpecificationController(stores *s.Stores, linter oaslint.Linter, whitelistedBasepaths, whitelistedCategories map[string]struct{}, errorMessage string) *ApiSpecificationController {
	ctrl := &ApiSpecificationController{
		stores:                stores,
		Store:                 stores.APISpecificationStore,
		ZoneStore:             stores.ZoneStore,
		Linter:                linter,
		WhitelistedBasepaths:  whitelistedBasepaths,
		WhitelistedCategories: whitelistedCategories,
		ErrorMessage:          errorMessage,
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

	// Validate the API category against the known ApiCategories.
	if catErr := a.validateApiCategory(ctx, apiSpec.Spec.Category); catErr != nil {
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

	// Check if linting can be skipped (whitelist or hash dedup) synchronously.
	// If the spec needs actual external linting, we dispatch it asynchronously.
	var lintCfg *adminv1.LintingConfig
	var needsAsyncLint bool
	if a.Linter != nil {
		lintCfg = a.lookupLintingConfig(ctx, id.Environment)
		needsAsyncLint = a.prepareLinting(ctx, lintCfg, apiSpec, specMarshaled)
	}

	err = a.Store.CreateOrReplace(ctx, apiSpec)
	if err != nil {
		return res, err
	}

	// Dispatch async linting if needed. The background goroutine will update the CRD status.
	if needsAsyncLint {
		a.dispatchAsyncLint(ctx, apiSpec.Namespace, apiSpec.Name, lintCfg, specMarshaled)
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

// prepareLinting checks whitelists and hash dedup synchronously.
// It returns true if an async external linter call is needed.
// If linting is not needed (disabled, whitelisted, or hash unchanged), it updates apiSpec in place and returns false.
func (a *ApiSpecificationController) prepareLinting(_ context.Context, lintCfg *adminv1.LintingConfig, apiSpec *roverv1.ApiSpecification, specBytes []byte) bool {
	if a.Linter == nil || lintCfg == nil || !lintCfg.Enabled {
		return false
	}

	// Check basepath whitelist (controller-config level).
	if _, ok := a.WhitelistedBasepaths[apiSpec.Spec.BasePath]; ok {
		log.Infof("Basepath %q is whitelisted, skipping linting", apiSpec.Spec.BasePath)
		passed := true
		apiSpec.Status.LintPassed = &passed
		apiSpec.Status.LintReason = fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)
		return false
	}

	// Check category whitelist (controller-config level).
	if _, ok := a.WhitelistedCategories[strings.ToLower(apiSpec.Spec.Category)]; ok {
		log.Infof("Category %q is whitelisted (controller config), skipping linting", apiSpec.Spec.Category)
		passed := true
		apiSpec.Status.LintPassed = &passed
		apiSpec.Status.LintReason = fmt.Sprintf("The category %q is whitelisted", apiSpec.Spec.Category)
		return false
	}

	// Check category whitelist (zone-level).
	if isCategoryWhitelistedByZone(lintCfg, apiSpec.Spec.Category) {
		log.Infof("Category %q is whitelisted (zone config), skipping linting", apiSpec.Spec.Category)
		passed := true
		apiSpec.Status.LintPassed = &passed
		apiSpec.Status.LintReason = fmt.Sprintf("The category %q is whitelisted by zone", apiSpec.Spec.Category)
		return false
	}

	// Hash dedup: skip re-linting if the spec content has not changed.
	specHash := computeHash(specBytes)
	if apiSpec.Status.LintedHash == specHash && apiSpec.Status.LintPassed != nil {
		log.Infof("Spec hash unchanged (%s), reusing previous lint result (passed=%v)", specHash, *apiSpec.Status.LintPassed)
		return false
	}

	// Mark as linting pending — the actual call happens asynchronously.
	apiSpec.Status.LintPassed = nil
	apiSpec.Status.LintReason = "Linting in progress"
	apiSpec.Status.LintedHash = ""
	return true
}

// dispatchAsyncLint runs the external linter call in a background goroutine.
// It updates the ApiSpecification CRD status with the lint result when done.
func (a *ApiSpecificationController) dispatchAsyncLint(ctx context.Context, ns, name string, lintCfg *adminv1.LintingConfig, specBytes []byte) {
	// Create a detached context so the background work is not cancelled when the HTTP request ends.
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		result, err := a.Linter.Lint(bgCtx, specBytes)
		if err != nil {
			log.Errorf("Async OAS linting failed for %s/%s: %v", ns, name, err)
			a.updateLintStatus(bgCtx, ns, name, lintCfg, &oaslint.LintResult{
				Passed: false,
				Reason: fmt.Sprintf("linter API error: %s", err),
			}, specBytes)
			return
		}

		a.updateLintStatus(bgCtx, ns, name, lintCfg, result, specBytes)
	}()
}

// updateLintStatus fetches the current ApiSpecification, updates its lint status fields, and writes it back.
func (a *ApiSpecificationController) updateLintStatus(ctx context.Context, ns, name string, lintCfg *adminv1.LintingConfig, result *oaslint.LintResult, specBytes []byte) {
	apiSpec, err := a.Store.Get(ctx, ns, name)
	if err != nil {
		log.Errorf("Failed to fetch ApiSpecification %s/%s for lint status update: %v", ns, name, err)
		return
	}

	specHash := computeHash(specBytes)
	passed := result.Passed
	apiSpec.Status.LintedHash = specHash
	apiSpec.Status.LintPassed = &passed
	apiSpec.Status.LintReason = result.Reason
	apiSpec.Status.LinterId = result.LinterId
	apiSpec.Status.LintRuleset = result.Ruleset
	apiSpec.Status.LintLinterVersion = result.LinterVersion
	apiSpec.Status.LintErrors = result.Errors
	apiSpec.Status.LintWarnings = result.Warnings

	// Populate the linter dashboard URL from zone config if available.
	if lintCfg != nil && lintCfg.DashboardURLTemplate != "" && result.LinterId != "" {
		apiSpec.Status.LintDashboardURL = strings.ReplaceAll(lintCfg.DashboardURLTemplate, "{linterId}", result.LinterId)
	}

	if !passed {
		message := strings.ReplaceAll(a.ErrorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
		apiSpec.Status.LintReason = message
		log.Infof("Async OAS linting failed for %s/%s: %s (errors=%d, warnings=%d)",
			ns, name, result.Reason, result.Errors, result.Warnings)
	}

	if err := a.Store.CreateOrReplace(ctx, apiSpec); err != nil {
		log.Errorf("Failed to update lint status for %s/%s: %v", ns, name, err)
	}
}

// isCategoryWhitelistedByZone checks if the given category is whitelisted in the zone-level linting config.
func isCategoryWhitelistedByZone(lintCfg *adminv1.LintingConfig, category string) bool {
	for _, wl := range lintCfg.WhitelistedCategories {
		if strings.EqualFold(wl, category) {
			return true
		}
	}
	return false
}

// computeHash returns the base64-encoded SHA-256 hash of the given data.
func computeHash(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

// validateApiCategory validates that the given category is a known and active ApiCategory.
// If ListApiCategories is nil, validation is skipped.
func (a *ApiSpecificationController) validateApiCategory(ctx context.Context, category string) error {
	if a.ListApiCategories == nil {
		return nil
	}

	apiCategoryList, err := a.ListApiCategories(ctx)
	if err != nil {
		log.Warnf("Failed to list ApiCategories for validation: %v", err)
		return nil
	}

	found, ok := apiCategoryList.FindByLabelValue(category)
	if !ok {
		allowedLabels := strings.Join(apiCategoryList.AllowedLabelValues(), ", ")
		return problems.BadRequest(
			fmt.Sprintf("ApiCategory %q not found. Allowed values are: [%s]", category, allowedLabels))
	}

	if !found.Spec.Active {
		return problems.BadRequest(
			fmt.Sprintf("ApiCategory %q is not active", category))
	}

	return nil
}

// lookupLintingConfig finds the linting configuration from the zones in the given environment.
// It returns the first zone's linting config that is enabled.
func (a *ApiSpecificationController) lookupLintingConfig(ctx context.Context, environment string) *adminv1.LintingConfig {
	if a.ZoneStore == nil {
		return nil
	}

	listOpts := store.NewListOpts()
	listOpts.Prefix = environment
	zoneList, err := a.ZoneStore.List(ctx, listOpts)
	if err != nil || zoneList == nil {
		return nil
	}

	for i := range zoneList.Items {
		zone := zoneList.Items[i]
		if zone.Spec.Linting != nil && zone.Spec.Linting.Enabled {
			return zone.Spec.Linting
		}
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
