// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package oaslint

import (
	"context"
	"io"

	"github.com/go-logr/logr"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ Linter = (*noopLinter)(nil)

// noopLinter always skips linting. Used when no linter URL is configured.
type noopLinter struct{}

func (n *noopLinter) Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, _ *apiv1.ApiCategory, _ io.Reader) (Outcome, error) {
	logr.FromContextOrDiscard(ctx).V(1).Info("Linter URL not configured, skipping")
	apiSpec.Spec.Lint = &roverv1.LintResult{Passed: true, Message: "linting is disabled"}
	return Skipped, nil
}
