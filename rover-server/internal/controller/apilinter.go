// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/internal/config"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// LintOutcome describes how linting completed.
type LintOutcome int

const (
	// LintSkipped means no linting was needed (no config, whitelisted, or hash dedup).
	LintSkipped LintOutcome = iota
	// LintCompleted means linting ran synchronously and the result is on apiSpec.Spec.Lint.
	LintCompleted
	// LintDispatched means the spec was stored and linting is running asynchronously.
	// The caller should NOT store the spec again.
	LintDispatched
)

// ApiLinter abstracts the full OAS linting lifecycle: config lookup,
// whitelists, hash dedup, sync/async execution, and store interaction.
type ApiLinter interface {
	// Lint performs the full linting lifecycle for an ApiSpecification.
	// It looks up the linting config from the category list, checks whitelists
	// and hash dedup, and either runs the linter synchronously or dispatches
	// it asynchronously.
	//
	// Returns the outcome and an error for infrastructure failures during sync linting.
	// When the outcome is LintDispatched, the spec has already been stored — the caller
	// must not store it again.
	Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, categoryList *apiv1.ApiCategoryList, specBytes []byte) (LintOutcome, error)

	// Shutdown waits for all in-flight async lint operations to complete.
	Shutdown()
}

// apiLinterImpl is the production implementation of ApiLinter.
type apiLinterImpl struct {
	objStore     store.ObjectStore[*roverv1.ApiSpecification]
	errorMessage string
	async        bool
	httpClient   oaslint.HTTPDoer
	wg           sync.WaitGroup
}

// NewApiLinter creates an ApiLinter from the given linting configuration.
func NewApiLinter(objStore store.ObjectStore[*roverv1.ApiSpecification], lintCfg config.OasLintingConfig) ApiLinter {
	return &apiLinterImpl{
		objStore:     objStore,
		errorMessage: lintCfg.ErrorMessage,
		async:        lintCfg.Async,
		httpClient: commonclient.NewHttpClientOrDie(
			commonclient.WithClientName("oaslint"),
			commonclient.WithClientTimeout(lintCfg.Timeout),
			commonclient.WithSkipTlsVerify(true),
		),
	}
}

func (l *apiLinterImpl) Shutdown() { l.wg.Wait() }

func (l *apiLinterImpl) Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, categoryList *apiv1.ApiCategoryList, specBytes []byte) (LintOutcome, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Looking up linting config", "namespace", apiSpec.Namespace, "name", apiSpec.Name,
		"category", apiSpec.Spec.Category, "basepath", apiSpec.Spec.BasePath)

	lintCfg := lintingConfigFromList(categoryList, apiSpec.Spec.Category)
	if lintCfg == nil || lintCfg.URL == "" || lintCfg.Mode == apiv1.LintingModeNone {
		log.V(1).Info("No linting config or no URL, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return LintSkipped, nil
	}

	log.V(1).Info("Linting config found, checking whitelists and hash dedup", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
	existing, _ := l.objStore.Get(ctx, apiSpec.Namespace, apiSpec.Name)
	if !l.prepareLinting(lintCfg, apiSpec, existing) {
		log.V(1).Info("Linting skipped (whitelisted or hash dedup)", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return LintSkipped, nil
	}

	if l.async {
		if err := l.objStore.CreateOrReplace(ctx, apiSpec); err != nil {
			return LintSkipped, err
		}
		l.dispatchAsyncLint(ctx, apiSpec.Namespace, apiSpec.Name, lintCfg.URL, lintCfg.Ruleset, specBytes)
		return LintDispatched, nil
	}

	if err := l.runSyncLint(ctx, apiSpec, lintCfg.URL, lintCfg.Ruleset, specBytes); err != nil {
		_ = l.objStore.CreateOrReplace(ctx, apiSpec)
		return LintCompleted, err
	}
	return LintCompleted, nil
}

func (l *apiLinterImpl) prepareLinting(lintCfg *apiv1.LintingConfig, apiSpec *roverv1.ApiSpecification, existing *roverv1.ApiSpecification) bool {
	if isBasepathWhitelisted(lintCfg, apiSpec.Spec.BasePath) {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)}
		return false
	}
	if existing != nil && existing.Spec.Lint != nil && existing.Spec.Hash == apiSpec.Spec.Hash {
		apiSpec.Spec.Lint = existing.Spec.Lint
		return false
	}
	apiSpec.Spec.Lint = nil
	return true
}

func (l *apiLinterImpl) runSyncLint(ctx context.Context, apiSpec *roverv1.ApiSpecification, linterURL, ruleset string, specBytes []byte) error {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")
	lintResult, err := l.executeLint(ctx, linterURL, ruleset, specBytes)
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

func (l *apiLinterImpl) dispatchAsyncLint(ctx context.Context, ns, name, linterURL, ruleset string, specBytes []byte) {
	bgCtx := context.WithoutCancel(ctx)
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		log := logr.FromContextOrDiscard(bgCtx).WithName("linting")

		lintResult, err := l.executeLint(bgCtx, linterURL, ruleset, specBytes)
		if err != nil {
			log.Error(err, "Async OAS linting failed", "namespace", ns, "name", name)
		} else if !lintResult.Passed {
			log.Info("Linting failed", "namespace", ns, "name", name, "message", lintResult.Message)
		}

		if _, patchErr := l.objStore.Patch(bgCtx, ns, name, store.Patch{
			Op:    store.OpReplace,
			Path:  "/spec/lint",
			Value: lintResult,
		}); patchErr != nil {
			log.Error(patchErr, "Failed to update lint result", "namespace", ns, "name", name)
		}
	}()
}

func (l *apiLinterImpl) executeLint(ctx context.Context, linterURL, ruleset string, specBytes []byte) (*roverv1.LintResult, error) {
	var opts []oaslint.ExternalLinterOption
	if ruleset != "" {
		opts = append(opts, oaslint.WithRuleset(ruleset))
	}
	opts = append(opts, oaslint.WithHTTPClient(l.httpClient))
	linter := oaslint.NewExternalLinter(linterURL, opts...)

	result, err := linter.Lint(ctx, specBytes)
	if err != nil {
		return &roverv1.LintResult{
			Passed:  false,
			Message: fmt.Sprintf("linter API error: %s", err),
		}, fmt.Errorf("linter API error: %w", err)
	}
	return l.buildLintResult(result, linterURL), nil
}

func (l *apiLinterImpl) buildLintResult(result *oaslint.LintResult, linterURL string) *roverv1.LintResult {
	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}
	if linterURL != "" && result.LinterId != "" {
		lintResult.DashboardURL = fmt.Sprintf("%s/scans/%s", strings.TrimRight(linterURL, "/"), result.LinterId)
	}
	if !result.Passed {
		lintResult.Message = strings.ReplaceAll(l.errorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
	}
	return lintResult
}
