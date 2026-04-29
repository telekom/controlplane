// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	scanEndpoint    = "api/linter/scans"
	yamlContentType = "application/yaml; charset=UTF-8"
)

var _ Linter = (*ExternalLinter)(nil)

// ExternalLinter calls an external linter REST API (Atlas Linter Service compatible).
// POST {baseURL}/api/linter/scans with the OAS spec as YAML body.
type ExternalLinter struct {
	baseURL string
	ruleset string
	client  *http.Client
}

// ExternalLinterOption configures the ExternalLinter.
type ExternalLinterOption func(*ExternalLinter)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(c *http.Client) ExternalLinterOption {
	return func(l *ExternalLinter) {
		l.client = c
	}
}

// WithRuleset sets the ruleset query parameter for linter scan requests.
func WithRuleset(ruleset string) ExternalLinterOption {
	return func(l *ExternalLinter) {
		l.ruleset = ruleset
	}
}

// WithTimeout overrides the default HTTP client timeout.
func WithTimeout(d time.Duration) ExternalLinterOption {
	return func(l *ExternalLinter) {
		l.client.Timeout = d
	}
}

// NewExternalLinter creates a new ExternalLinter targeting the given base URL.
func NewExternalLinter(baseURL string, opts ...ExternalLinterOption) *ExternalLinter {
	l := &ExternalLinter{
		baseURL: baseURL,
		client:  &http.Client{},
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// linterScanResponse mirrors the external linter API response (Atlas Linter Service).
type linterScanResponse struct {
	ID            string         `json:"id"`
	CreatedAt     string         `json:"createdAt"`
	Ruleset       linterRuleset  `json:"ruleset"`
	Info          violationsInfo `json:"info"`
	LinterVersion string         `json:"linterVersion"`
}

type linterRuleset struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	URL  string `json:"url,omitempty"`
}

type violationsInfo struct {
	Infos    int `json:"infos"`
	Warnings int `json:"warnings"`
	Errors   int `json:"errors"`
	Hints    int `json:"hints"`
}

func (l *ExternalLinter) Lint(ctx context.Context, spec []byte) (*LintResult, error) {
	scanURL := fmt.Sprintf("%s/%s", l.baseURL, scanEndpoint)
	if l.ruleset != "" {
		scanURL = fmt.Sprintf("%s?ruleset=%s", scanURL, l.ruleset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scanURL, bytes.NewReader(spec))
	if err != nil {
		return nil, fmt.Errorf("creating linter request: %w", err)
	}
	req.Header.Set("Content-Type", yamlContentType)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling linter API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading linter response: %w", err)
	}

	if resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("linting timed out (HTTP 408)")
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("linter service unavailable (HTTP %d)", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("linter API returned unexpected status %d", resp.StatusCode)
	}

	var scan linterScanResponse
	if err := json.Unmarshal(body, &scan); err != nil {
		return nil, fmt.Errorf("decoding linter response: %w", err)
	}

	passed := scan.Info.Errors == 0
	reason := "linter scan result does not contain errors"
	if !passed {
		reason = fmt.Sprintf("linter scan found %d error(s) per %q rules", scan.Info.Errors, scan.Ruleset.Name)
	}

	return &LintResult{
		Passed:   passed,
		Reason:   reason,
		Ruleset:  scan.Ruleset.Name,
		LinterId: scan.ID,
		Errors:   scan.Info.Errors,
		Warnings: scan.Info.Warnings,
	}, nil
}
