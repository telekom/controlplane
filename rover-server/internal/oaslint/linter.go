// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-logr/logr"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	client "github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"github.com/telekom/controlplane/rover-server/internal/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// Outcome describes how linting completed.
type Outcome int

const (
	// Skipped means no linting was needed (no config, whitelisted, or noop).
	Skipped Outcome = iota
	// Completed means linting ran synchronously and the result is on apiSpec.Spec.Lint.
	Completed
	// Blocked means linting ran, the spec failed, and the category mode is Block.
	Blocked
)

const (
	placeholderLinterId     = "{{.LinterId}}"
	placeholderRulesetName  = "{{.RulesetName}}"
	placeholderDashboardURL = "{{.DashboardURL}}"
)

// Linter defines the interface for OAS specification linting.
// Implementations handle the full lifecycle: config lookup, whitelists, and execution.
// They populate apiSpec.Spec.Lint with the result when linting is performed.
type Linter interface {
	Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, category *apiv1.ApiCategory, spec io.Reader) (Outcome, error)
}

// scanResult contains the raw outcome of a linter scan (internal to the package).
type scanResult struct {
	Passed   bool
	Reason   string
	Ruleset  string
	LinterId string
	Errors   int
	Warnings int
}

// Option configures the Linter construction.
type Option func(*linterImpl)

// WithHTTPClient sets a custom HTTP client on the linter.
func WithHTTPClient(c client.HttpRequestDoer) Option {
	return func(l *linterImpl) {
		l.httpClient = c
	}
}

// NewLinter creates a Linter from the given OasLintingConfig.
// If URL is empty, a noop linter is returned that always skips.
func NewLinter(cfg config.OasLintingConfig, opts ...Option) Linter {
	if cfg.URL == "" {
		return &noopLinter{}
	}
	l := &linterImpl{
		errorMessageTemplate: cfg.ErrorMessage,
		url:                  cfg.URL,
		dashboardURL:         cfg.DashboardURL,
		httpClient: commonclient.NewHttpClientOrDie(
			commonclient.WithClientName("oaslint"),
			commonclient.WithClientTimeout(cfg.Timeout),
			commonclient.WithSkipTlsVerify(cfg.SkipTLS),
		),
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

var _ Linter = (*linterImpl)(nil)

// linterImpl is the production implementation of Linter.
type linterImpl struct {
	errorMessageTemplate string
	url                  string
	dashboardURL         string
	httpClient           client.HttpRequestDoer
}

func (l *linterImpl) Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, category *apiv1.ApiCategory, specBytes io.Reader) (Outcome, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Looking up linting config", "namespace", apiSpec.Namespace, "name", apiSpec.Name,
		"category", apiSpec.Spec.Category, "basepath", apiSpec.Spec.BasePath)

	if category == nil {
		log.V(1).Info("No category provided, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return Skipped, nil
	}

	lintCfg := category.Spec.Linting
	if lintCfg == nil || lintCfg.Mode == apiv1.LintingModeNone {
		log.V(1).Info("No linting config, skipping linting", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return Skipped, nil
	}

	log.V(1).Info("Linting config found, checking whitelists", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
	if !l.prepareLinting(lintCfg, apiSpec) {
		log.V(1).Info("Linting skipped (whitelisted)", "namespace", apiSpec.Namespace, "name", apiSpec.Name)
		return Skipped, nil
	}

	if err := l.runLint(ctx, apiSpec, lintCfg.Ruleset, specBytes); err != nil {
		return Completed, err
	}

	if lintCfg.Mode == apiv1.LintingModeBlock && apiSpec.Spec.Lint != nil && !apiSpec.Spec.Lint.Passed {
		return Blocked, nil
	}

	return Completed, nil
}

func (l *linterImpl) prepareLinting(lintCfg *apiv1.LintingConfig, apiSpec *roverv1.ApiSpecification) bool {
	if lintCfg.IsBasepathWhitelisted(apiSpec.Spec.BasePath) {
		apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: fmt.Sprintf("The basepath %q is whitelisted", apiSpec.Spec.BasePath)}
		return false
	}
	apiSpec.Spec.Lint = nil
	return true
}

func (l *linterImpl) runLint(ctx context.Context, apiSpec *roverv1.ApiSpecification, ruleset string, specBytes io.Reader) error {
	log := logr.FromContextOrDiscard(ctx).WithName("linting")

	var opts []externalLinterOption
	if ruleset != "" {
		opts = append(opts, withRuleset(ruleset))
	}
	opts = append(opts, withHTTPClient(l.httpClient))
	linter := newExternalLinter(l.url, opts...)

	// Use a detached context so the linter call is not cancelled by the
	// Fiber request timeout. The HTTP client's own timeout governs how long
	// we wait.
	lintCtx := context.WithoutCancel(ctx)
	result, err := linter.lint(lintCtx, specBytes)
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

func (l *linterImpl) buildLintResult(result *scanResult) *roverv1.LintResult {
	lintResult := &roverv1.LintResult{
		Passed:  result.Passed,
		Message: result.Reason,
	}
	if l.dashboardURL != "" {
		url := l.dashboardURL
		url = strings.ReplaceAll(url, placeholderLinterId, result.LinterId)
		url = strings.ReplaceAll(url, placeholderRulesetName, result.Ruleset)
		lintResult.DashboardURL = url
	}
	if !result.Passed {
		msg := strings.ReplaceAll(l.errorMessageTemplate, placeholderRulesetName, result.Ruleset)
		msg = strings.ReplaceAll(msg, placeholderDashboardURL, lintResult.DashboardURL)
		lintResult.Message = msg
	}
	return lintResult
}
