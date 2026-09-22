// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// export_test.go exposes unexported migration types and methods to external
// tests (package handler_test). This file is compiled only during testing.

package handler

import (
	"context"

	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// AuthorizationIntent is the exported alias for authorizationIntent (test use).
type AuthorizationIntent = authorizationIntent

// DualApprovalResult is the exported alias for dualApprovalResult (test use).
type DualApprovalResult = dualApprovalResult

// Outcome constants for test use.
const (
	OutcomeGranted = outcomeGranted
	OutcomePending = outcomePending
)

// NewDualApprovalResult creates a dualApprovalResult with the given outcome for tests.
func NewDualApprovalResult(outcome approvalOutcome, err error) *dualApprovalResult {
	return &dualApprovalResult{
		outcome: outcome,
		err:     err,
	}
}

// NewTestIntent creates a minimal authorizationIntent for migration tests.
func NewTestIntent() authorizationIntent {
	return authorizationIntent{
		PolicyVersion: "v2",
		ConsumerName:  "consumer-app",
		ConsumerTeam:  "team-alpha",
		ProviderName:  "provider-app",
		ProviderTeam:  "team-beta",
		ApiBasePath:   "/api/v1/orders",
	}
}

// IsFreshInstall is the exported wrapper for isFreshInstall (test use).
func (h *ListenerHandler) IsFreshInstall(ctx context.Context, listener *spectrev1.Listener) (bool, error) {
	return h.isFreshInstall(ctx, listener)
}

// AdvanceMigration is the exported wrapper for advanceMigration (test use).
func (h *ListenerHandler) AdvanceMigration(
	ctx context.Context,
	listener *spectrev1.Listener,
	intent *authorizationIntent,
	dual *dualApprovalResult,
) (bool, error) {
	return h.advanceMigration(ctx, listener, intent, dual)
}

// IsLegacyBlocked is the exported wrapper for isLegacyBlocked (test use).
func IsLegacyBlocked(approval *approvalapi.Approval) (bool, string) {
	return isLegacyBlocked(approval)
}
