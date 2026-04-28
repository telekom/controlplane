// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import "context"

// Linter defines the interface for OAS specification linting.
// The external linter server manages rulesets; clients just send the spec.
type Linter interface {
	Lint(ctx context.Context, spec []byte) (*LintResult, error)
}

// LintResult contains the outcome of a linting operation.
type LintResult struct {
	Passed        bool
	Reason        string
	Ruleset       string
	LinterVersion string
	LinterId      string
	Errors        int
	Warnings      int
	Hints         int
	Infos         int
}
