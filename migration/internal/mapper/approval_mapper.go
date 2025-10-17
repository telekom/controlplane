// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"context"
	"time"

	"github.com/pkg/errors"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
)

// ApprovalMapper maps legacy Approval to ApprovalRequest
type ApprovalMapper struct{}

func NewApprovalMapper() *ApprovalMapper {
	return &ApprovalMapper{}
}

// MapApprovalToRequest converts legacy Approval state to ApprovalRequest
func (m *ApprovalMapper) MapApprovalToRequest(
	ctx context.Context,
	approvalRequest *approvalv1.ApprovalRequest,
	legacyApproval *approvalv1.Approval,
) error {
	legacyState := legacyApproval.Spec.State

	if !m.isValidState(legacyState) {
		return errors.Errorf("invalid legacy state: %s", legacyState)
	}

	// Map state with special handling for Suspended
	mappedState := m.MapState(legacyState)
	approvalRequest.Spec.State = mappedState

	// Map strategy
	if m.isValidStrategy(legacyApproval.Spec.Strategy) {
		approvalRequest.Spec.Strategy = legacyApproval.Spec.Strategy
	}

	// Add migration annotations
	if approvalRequest.Annotations == nil {
		approvalRequest.Annotations = make(map[string]string)
	}

	approvalRequest.Annotations["migration.cp.ei.telekom.de/migrated-from"] = legacyApproval.Name
	approvalRequest.Annotations["migration.cp.ei.telekom.de/migration-timestamp"] = time.Now().Format(time.RFC3339)
	approvalRequest.Annotations["migration.cp.ei.telekom.de/last-migrated-state"] = string(mappedState)
	approvalRequest.Annotations["migration.cp.ei.telekom.de/legacy-state"] = string(legacyState)

	return nil
}

// MapState handles special state mappings between legacy and new cluster
func (m *ApprovalMapper) MapState(legacyState approvalv1.ApprovalState) approvalv1.ApprovalState {
	// Special mapping: Suspended in legacy cluster -> Rejected in new cluster
	if legacyState == approvalv1.ApprovalStateSuspended {
		return approvalv1.ApprovalStateRejected
	}

	// All other states map directly
	return legacyState
}

// isValidState checks if the state is valid
func (m *ApprovalMapper) isValidState(state approvalv1.ApprovalState) bool {
	validStates := []approvalv1.ApprovalState{
		approvalv1.ApprovalStatePending,
		approvalv1.ApprovalStateGranted,
		approvalv1.ApprovalStateRejected,
		approvalv1.ApprovalStateSuspended,
		approvalv1.ApprovalStateSemigranted,
	}

	for _, validState := range validStates {
		if state == validState {
			return true
		}
	}
	return false
}

// isValidStrategy checks if the strategy is valid
func (m *ApprovalMapper) isValidStrategy(strategy approvalv1.ApprovalStrategy) bool {
	if strategy == "" {
		return false
	}

	validStrategies := []approvalv1.ApprovalStrategy{
		approvalv1.ApprovalStrategySimple,
		approvalv1.ApprovalStrategyFourEyes,
	}

	for _, validStrategy := range validStrategies {
		if strategy == validStrategy {
			return true
		}
	}
	return false
}
