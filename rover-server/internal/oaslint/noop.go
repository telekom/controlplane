// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import "context"

var _ Linter = (*NoopLinter)(nil)

// NoopLinter always returns a passing result. Used when linting is disabled.
type NoopLinter struct{}

func (n *NoopLinter) Lint(_ context.Context, _ []byte) (*LintResult, error) {
	return &LintResult{
		Passed: true,
		Reason: "linting is disabled",
	}, nil
}
