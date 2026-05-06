// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	pkglog "github.com/telekom/controlplane/rover-server/pkg/log"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// prepareLinting checks whitelists and hash dedup synchronously.
// It returns true if an external linter call is needed.
func (a *ApiSpecificationController) prepareLinting(lintCfg *apiv1.LintingConfig, apiSpec *roverv1.ApiSpecification, existing *roverv1.ApiSpecification) bool {
	log := pkglog.Log.WithName("linting")

	// Check basepath whitelist (category-level).
	if isBasepathWhitelisted(lintCfg, apiSpec.Spec.BasePath) {
		log.Info("Basepath is whitelisted, skipping linting", "basepath", apiSpec.Spec.BasePath)
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)}
		return false
	}

	// Hash dedup: if the spec content hasn't changed and a previous lint result exists, reuse it.
	if existing != nil && existing.Spec.Lint != nil && existing.Spec.Hash == apiSpec.Spec.Hash {
		log.Info("Spec hash unchanged, reusing previous lint result", "passed", existing.Spec.Lint.Passed)
		apiSpec.Spec.Lint = existing.Spec.Lint
		return false
	}

	// Clear previous result — the actual linter call will follow.
	apiSpec.Spec.Lint = nil
	return true
}

// isBasepathWhitelisted checks if the given basepath is whitelisted in the category-level linting config.
func isBasepathWhitelisted(lintCfg *apiv1.LintingConfig, basepath string) bool {
	for _, wl := range lintCfg.WhitelistedBasepaths {
		if strings.EqualFold(wl, basepath) {
			return true
		}
	}
	return false
}

// runSyncLint calls the external linter synchronously and sets the lint result directly on the apiSpec.
// This ensures the lint result is included in the same store write as the spec itself.
// It returns an error for infrastructure failures (e.g. linter unreachable or auth errors)
// which should be surfaced as 500 Internal Server Error to the client.
func (a *ApiSpecificationController) runSyncLint(ctx context.Context, apiSpec *roverv1.ApiSpecification, linterURL, ruleset string, specBytes []byte) error {
	log := pkglog.Log.WithName("linting")
	var opts []oaslint.ExternalLinterOption
	if ruleset != "" {
		opts = append(opts, oaslint.WithRuleset(ruleset))
	}
	if a.LintTimeout > 0 {
		opts = append(opts, oaslint.WithTimeout(a.LintTimeout))
	}
	linter := oaslint.NewExternalLinter(linterURL, opts...)

	result, err := linter.Lint(ctx, specBytes)
	if err != nil {
		log.Error(err, "Sync OAS linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		apiSpec.Spec.Lint = &roverv1.LintResult{
			Passed:  false,
			Message: fmt.Sprintf("linter API error: %s", err),
		}
		return fmt.Errorf("linter API error: %w", err)
	}

	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}
	if linterURL != "" && result.LinterId != "" {
		lintResult.DashboardURL = fmt.Sprintf("%s/scans/%s", strings.TrimRight(linterURL, "/"), result.LinterId)
	}
	if !result.Passed {
		lintResult.Message = strings.ReplaceAll(a.ErrorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
		log.Info("Linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name,
			"reason", result.Reason, "errors", result.Errors, "warnings", result.Warnings)
	}
	apiSpec.Spec.Lint = lintResult
	return nil
}

// dispatchAsyncLint runs the external linter call in a background goroutine.
// It updates the ApiSpecification CRD spec with the lint result when done.
func (a *ApiSpecificationController) dispatchAsyncLint(ctx context.Context, ns, name, linterURL, ruleset string, specBytes []byte) {
	// Create a detached context so the background work is not cancelled when the HTTP request ends.
	bgCtx := context.WithoutCancel(ctx)
	var opts []oaslint.ExternalLinterOption
	if ruleset != "" {
		opts = append(opts, oaslint.WithRuleset(ruleset))
	}
	if a.LintTimeout > 0 {
		opts = append(opts, oaslint.WithTimeout(a.LintTimeout))
	}
	linter := oaslint.NewExternalLinter(linterURL, opts...)
	go func() {
		log := pkglog.Log.WithName("linting")
		result, err := linter.Lint(bgCtx, specBytes)
		if err != nil {
			log.Error(err, "Async OAS linting failed", "namespace", ns, "name", name)
			a.updateLintResult(bgCtx, ns, name, linterURL, &oaslint.LintResult{
				Passed: false,
				Reason: fmt.Sprintf("linter API error: %s", err),
			})
			return
		}

		a.updateLintResult(bgCtx, ns, name, linterURL, result)
	}()
}

// updateLintResult patches the ApiSpecification's Spec.Lint field with the linting result.
func (a *ApiSpecificationController) updateLintResult(ctx context.Context, ns, name, linterURL string, result *oaslint.LintResult) {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")

	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}

	// Build the linter dashboard URL from the linter-api base URL and the scan ID.
	if linterURL != "" && result.LinterId != "" {
		lintResult.DashboardURL = fmt.Sprintf("%s/scans/%s", strings.TrimRight(linterURL, "/"), result.LinterId)
	}

	if !result.Passed {
		lintResult.Message = strings.ReplaceAll(a.ErrorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
		log.Info("Linting failed", "namespace", ns, "name", name,
			"reason", result.Reason, "errors", result.Errors, "warnings", result.Warnings)
	}

	if _, err := a.Store.Patch(ctx, ns, name, store.Patch{
		Op:    store.OpReplace,
		Path:  "/spec/lint",
		Value: lintResult,
	}); err != nil {
		log.Error(err, "Failed to update lint result", "namespace", ns, "name", name)
	}
}

// lookupLintingConfig finds the linting configuration from the ApiCategory matching the given category.
// Returns nil if no linting is configured for this category.
func (a *ApiSpecificationController) lookupLintingConfig(ctx context.Context, category string) *apiv1.LintingConfig {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")

	if a.ListApiCategories == nil {
		log.V(1).Info("ListApiCategories is nil, skipping linting lookup", "category", category)
		return nil
	}

	categoryList, err := a.ListApiCategories(ctx)
	if err != nil {
		log.Error(err, "Failed to list ApiCategories for linting lookup")
		return nil
	}

	log.V(1).Info("Looking up linting config", "category", category, "categoryCount", len(categoryList.Items))

	found, ok := categoryList.FindByLabelValue(category)
	if !ok {
		log.V(1).Info("Category not found in ApiCategory list", "category", category)
		return nil
	}

	if found.Spec.Linting == nil {
		log.V(1).Info("Category found but has no linting config", "category", category)
		return nil
	}

	log.V(1).Info("Linting config resolved", "category", category,
		"url", found.Spec.Linting.URL, "ruleset", found.Spec.Linting.Ruleset,
		"mode", found.Spec.Linting.Mode, "whitelistedBasepaths", found.Spec.Linting.WhitelistedBasepaths)
	return found.Spec.Linting
}
