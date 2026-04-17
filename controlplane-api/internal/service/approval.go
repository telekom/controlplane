// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// ApprovalService defines operations for managing Approval and ApprovalRequest resources.
type ApprovalService interface {
	DecideApprovalRequest(ctx context.Context, input model.DecideApprovalRequestInput) (*model.ApprovalMutationResult, error)
	DecideApproval(ctx context.Context, input model.DecideApprovalInput) (*model.ApprovalMutationResult, error)
}
