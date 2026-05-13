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
	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
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
	// LintBlocked means linting ran, the spec failed, and the category mode is Block.
	LintBlocked
)

// ApiLinter abstracts the full OAS linting lifecycle: config lookup,
// whitelists, and execution and should populate apiSpec.Spec.Lint with the result if linting was performed.
type ApiLinter interface {
	// Lint performs the full linting lifecycle for an ApiSpecification.
	// It looks up the linting config from the category list, checks whitelists,
	// and runs the linter synchronously.
	Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, category *apiv1.ApiCategory, specBytes []byte) (LintOutcome, error)
}

// apiLinterImpl is the production implementation of ApiLinter.
type apiLinterImpl struct {
	errorMessage string
	url          string
	dashboardURL string
	httpClient   oaslint.HTTPDoer
}

// NewApiLinter creates an ApiLinter from the given linting configuration.
func NewApiLinter(lintCfg config.OasLintingConfig) ApiLinter {
	return &apiLinterImpl{
		errorMessage: lintCfg.ErrorMessage,
		url:          lintCfg.URL,
		dashboardURL: lintCfg.DashboardURL,
		httpClient: commonclient.NewHttpClientOrDie(
			commonclient.WithClientName("oaslint"),
			commonclient.WithClientTimeout(lintCfg.Timeout),
			commonclient.WithSkipTlsVerify(true),
		),
	}
}

func (l *apiLinterImpl) Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, category *apiv1.ApiCategory, specBytes []byte) (LintOutcome, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Looking up linting config", "namespace", apiSpec.Namespace, "name", apiSpec.Name,
		"category", apiSpec.Spec.Category, "basepath", apiSpec.Spec.BasePath)

	if category == nil {
		log.V(1).Info("No category provided, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return LintSkipped, nil
	}

	lintCfg := category.Spec.Linting
	if lintCfg == nil || l.url == "" || lintCfg.Mode == apiv1.LintingModeNone {
		log.V(1).Info("No linting config or no URL, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return LintSkipped, nil
	}

	log.V(1).Info("Linting config found, checking whitelists", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
	if !l.prepareLinting(lintCfg, apiSpec) {
		log.V(1).Info("Linting skipped (whitelisted)", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return LintSkipped, nil
	}

	if err := l.runLint(ctx, apiSpec, lintCfg.Ruleset, specBytes); err != nil {
		return LintCompleted, err
	}

	if lintCfg.Mode == apiv1.LintingModeBlock && apiSpec.Spec.Lint != nil && !apiSpec.Spec.Lint.Passed {
		return LintBlocked, fmt.Errorf("linting failed in block mode: %s", apiSpec.Spec.Lint.Message)
	}

	return LintCompleted, nil
}

func (l *apiLinterImpl) prepareLinting(lintCfg *apiv1.LintingConfig, apiSpec *roverv1.ApiSpecification) bool {
	if isBasepathWhitelisted(lintCfg, apiSpec.Spec.BasePath) {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)}
		return false
	}
	apiSpec.Spec.Lint = nil
	return true
}

func (l *apiLinterImpl) runLint(ctx context.Context, apiSpec *roverv1.ApiSpecification, ruleset string, specBytes []byte) error {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")

	var opts []oaslint.ExternalLinterOption
	if ruleset != "" {
		opts = append(opts, oaslint.WithRuleset(ruleset))
	}
	opts = append(opts, oaslint.WithHTTPClient(l.httpClient))
	linter := oaslint.NewExternalLinter(l.url, opts...)

	result, err := linter.Lint(ctx, specBytes)
	if err != nil {
		apiSpec.Spec.Lint = &roverv1.LintResult{
			Passed:  false,
			Message: fmt.Sprintf("linter API error: %s", err),
		}
		log.Error(err, "OAS linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return fmt.Errorf("linter API error: %w", err)
	}

	apiSpec.Spec.Lint = l.buildLintResult(result)
	if !apiSpec.Spec.Lint.Passed {
		log.Info("Linting failed", "namespace", apiSpec.Namespace, "name", apiSpec.Name, "message", apiSpec.Spec.Lint.Message)
	}
	return nil
}

func (l *apiLinterImpl) buildLintResult(result *oaslint.LintResult) *roverv1.LintResult {
	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}
	if l.dashboardURL != "" && result.LinterId != "" {
		lintResult.DashboardURL = fmt.Sprintf("%s/scans/%s", strings.TrimRight(l.dashboardURL, "/"), result.LinterId)
	}
	if !result.Passed {
		lintResult.Message = strings.ReplaceAll(l.errorMessage, "RULESET_NAME_PLACEHOLDER", result.Ruleset)
	}
	return lintResult
}

// isBasepathWhitelisted checks whether the given basepath is in the category's whitelist.
func isBasepathWhitelisted(cfg *apiv1.LintingConfig, basepath string) bool {
	for _, wp := range cfg.WhitelistedBasepaths {
		if strings.EqualFold(wp, basepath) {
			return true
		}
	}
	return false
}
