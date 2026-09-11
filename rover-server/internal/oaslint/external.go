// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
	client "github.com/telekom/controlplane/common-server/pkg/client/metrics"
)

const (
	scanEndpoint    = "api/linter/scans"
	yamlContentType = "application/yaml; charset=UTF-8"
)

// HTTPDoer is the interface for executing HTTP requests.
// Compatible with *http.Client and metrics-wrapped clients.
type HTTPDoer = client.HttpRequestDoer

// externalLinter calls an external linter REST API (Atlas Linter Service compatible).
// POST {baseURL}/api/linter/scans with the OAS spec as YAML body.
type externalLinter struct {
	baseURL string
	ruleset string
	client  client.HttpRequestDoer
}

// externalLinterOption configures the externalLinter.
type externalLinterOption func(*externalLinter)

// withHTTPClient overrides the default HTTP client.
func withHTTPClient(c client.HttpRequestDoer) externalLinterOption {
	return func(l *externalLinter) {
		l.client = c
	}
}

// withRuleset sets the ruleset query parameter for linter scan requests.
func withRuleset(ruleset string) externalLinterOption {
	return func(l *externalLinter) {
		l.ruleset = ruleset
	}
}

// newExternalLinter creates a new externalLinter targeting the given base URL.
func newExternalLinter(baseURL string, opts ...externalLinterOption) *externalLinter {
	l := &externalLinter{
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

func (l *externalLinter) lint(ctx context.Context, spec io.Reader) (*scanResult, error) {
	scanURL := fmt.Sprintf("%s/%s", l.baseURL, scanEndpoint)
	if l.ruleset != "" {
		scanURL = fmt.Sprintf("%s?ruleset=%s", scanURL, url.QueryEscape(l.ruleset))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scanURL, spec)
	if err != nil {
		return nil, fmt.Errorf("creating linter request: %w", err)
	}
	req.Header.Set("Content-Type", yamlContentType)

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling linter API: %w", err)
	}
	defer resp.Body.Close()

	if err := commonclient.HandleError(resp.StatusCode, "linter API"); err != nil {
		return nil, fmt.Errorf("linter API error: %w", err)
	}

	var scan linterScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&scan); err != nil {
		return nil, fmt.Errorf("decoding linter response: %w", err)
	}

	passed := scan.Info.Errors == 0
	reason := "linter scan result does not contain errors"
	if !passed {
		reason = fmt.Sprintf("linter scan found %d error(s) per %q rules", scan.Info.Errors, scan.Ruleset.Name)
	}

	return &scanResult{
		Passed:   passed,
		Reason:   reason,
		Ruleset:  scan.Ruleset.Name,
		LinterId: scan.ID,
		Errors:   scan.Info.Errors,
		Warnings: scan.Info.Warnings,
	}, nil
}
