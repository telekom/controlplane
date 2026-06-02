// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"

	"github.com/go-logr/logr"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// noopApiLinter is used when no linter URL is configured globally. It always skips linting.
type noopApiLinter struct {
	noop oaslint.NoopLinter
}

func (n *noopApiLinter) Lint(ctx context.Context, apiSpec *roverv1.ApiSpecification, _ *apiv1.ApiCategory, specBytes io.Reader) (LintOutcome, error) {
	logr.FromContextOrDiscard(ctx).V(1).Info("Linter URL not configured, skipping")
	result, _ := n.noop.Lint(ctx, specBytes)
	apiSpec.Spec.Lint = &roverv1.LintResult{Passed: result.Passed, Message: result.Reason}
	return LintSkipped, nil
}
