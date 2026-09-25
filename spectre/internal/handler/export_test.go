// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// export_test.go exposes unexported types and methods to external
// tests (package handler_test). This file is compiled only during testing.

package handler

import (
	"context"
	"fmt"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// VerifyProviderBinding is the exported wrapper for verifyProviderBinding (test use).
func (h *ListenerHandler) VerifyProviderBinding(
	ctx context.Context,
	route *gatewayv1.Route,
	providerApp *applicationv1.Application,
) (*ProviderBinding, error) {
	return h.verifyProviderBinding(ctx, route, providerApp)
}

// StartDrain is the exported wrapper for startDrain (test use).
func (h *ListenerHandler) StartDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	oldFingerprint string,
) error {
	return h.startDrain(ctx, listener, reason, oldFingerprint)
}

// ContinueDrain is the exported wrapper for continueDrain (test use).
func (h *ListenerHandler) ContinueDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
) (bool, error) {
	return h.continueDrain(ctx, listener)
}

// Drain phase constants for test use.
const (
	ExportDrainPhaseStopping            = DrainPhaseStopping
	ExportDrainPhaseDrainingSubscribers = DrainPhaseDrainingSubscribers
	ExportDrainPhaseCleaningPublisher   = DrainPhaseCleaningPublisher
	ExportDrainPhaseComplete            = DrainPhaseComplete
)

// CheckEarlyRestriction is the exported wrapper for checkEarlyRestriction (test use).
func (h *ListenerHandler) CheckEarlyRestriction(
	ctx context.Context,
	listener *spectrev1.Listener,
) (approvalGate, requestGate string, err error) {
	return h.checkEarlyRestriction(ctx, listener)
}

// HandleDenialCleanup is the exported wrapper for handleDenialCleanup with the
// early Approval revocation reason and message (test use).
func (h *ListenerHandler) HandleDenialCleanup(
	ctx context.Context,
	listener *spectrev1.Listener,
	gateKey string,
) error {
	return h.handleDenialCleanup(ctx, listener,
		fmt.Sprintf("early restriction (%s gate)", gateKey),
		fmt.Sprintf("Approval has been revoked (%s gate, early restriction)", gateKey))
}
