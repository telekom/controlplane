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
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// prepareLinting checks whitelists and hash dedup synchronously.
// It returns true if an external linter call is needed.
func (a *ApiSpecificationController) prepareLinting(lintCfg *apiv1.LintingConfig, apiSpec *roverv1.ApiSpecification, existing *roverv1.ApiSpecification) bool {
	// Check basepath whitelist (category-level).
	if isBasepathWhitelisted(lintCfg, apiSpec.Spec.BasePath) {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)}
		return false
	}

	// Hash dedup: if the spec content hasn't changed and a previous lint result exists, reuse it.
	if existing != nil && existing.Spec.Lint != nil && existing.Spec.Hash == apiSpec.Spec.Hash {
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

// executeLint calls the external linter and returns the CRD lint result.
// This is the single lint execution path used by both sync and async flows.
func (a *ApiSpecificationController) executeLint(ctx context.Context, linterURL, ruleset string, specBytes []byte) (*roverv1.LintResult, error) {
	var opts []oaslint.ExternalLinterOption
	if ruleset != "" {
		opts = append(opts, oaslint.WithRuleset(ruleset))
	}
	opts = append(opts, oaslint.WithHTTPClient(a.httpClient))
	linter := oaslint.NewExternalLinter(linterURL, opts...)

	result, err := linter.Lint(ctx, specBytes)
	if err != nil {
		return &roverv1.LintResult{
			Passed:  false,
			Message: fmt.Sprintf("linter API error: %s", err),
		}, fmt.Errorf("linter API error: %w", err)
	}

	return a.buildLintResult(result, linterURL), nil
}

// runSyncLint calls the external linter synchronously and sets the lint result directly on the apiSpec.
// It returns an error for infrastructure failures (e.g. linter unreachable or auth errors)
// which should be surfaced as 500 Internal Server Error to the client.
func (a *ApiSpecificationController) runSyncLint(ctx context.Context, apiSpec *roverv1.ApiSpecification, linterURL, ruleset string, specBytes []byte) error {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")

	lintResult, err := a.executeLint(ctx, linterURL, ruleset, specBytes)
	apiSpec.Spec.Lint = lintResult
	if err != nil {
		log.Error(err, "Sync OAS linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return err
	}
	if !lintResult.Passed {
		log.Info("Linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name, "message", lintResult.Message)
	}
	return nil
}

// dispatchAsyncLint runs the lint call in a tracked background goroutine.
// It updates the ApiSpecification via store patch when done.
func (a *ApiSpecificationController) dispatchAsyncLint(ctx context.Context, ns, name, linterURL, ruleset string, specBytes []byte) {
	bgCtx := context.WithoutCancel(ctx)
	a.lintWg.Add(1)
	go func() {
		defer a.lintWg.Done()
		log := logr.FromContextOrDiscard(bgCtx).WithName("linting")

		lintResult, err := a.executeLint(bgCtx, linterURL, ruleset, specBytes)
		if err != nil {
			log.Error(err, "Async OAS linting failed", "namespace", ns, "name", name)
		} else if !lintResult.Passed {
			log.Info("Linting failed", "namespace", ns, "name", name, "message", lintResult.Message)
		}

		if _, patchErr := a.Store.Patch(bgCtx, ns, name, store.Patch{
			Op:    store.OpReplace,
			Path:  "/spec/lint",
			Value: lintResult,
		}); patchErr != nil {
			log.Error(patchErr, "Failed to update lint result", "namespace", ns, "name", name)
		}
	}()
}

// buildLintResult maps an oaslint.LintResult to the CRD's roverv1.LintResult,
// including dashboard URL and error message template substitution.
func (a *ApiSpecificationController) buildLintResult(result *oaslint.LintResult, linterURL string) *roverv1.LintResult {
	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}
	if linterURL != "" && result.LinterId != "" {
		lintResult.DashboardURL = fmt.Sprintf("%s/scans/%s", strings.TrimRight(linterURL, "/"), result.LinterId)
	}
	if !result.Passed {
		lintResult.Message = strings.ReplaceAll(a.ErrorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
	}
	return lintResult
}

// lintingConfigFromList finds the linting configuration from a pre-fetched ApiCategoryList.
// Returns nil if no linting is configured for this category.
func lintingConfigFromList(categoryList *apiv1.ApiCategoryList, category string) *apiv1.LintingConfig {
	if categoryList == nil {
		return nil
	}

	found, ok := categoryList.FindByLabelValue(category)
	if !ok {
		return nil
	}

	return found.Spec.Linting
}
