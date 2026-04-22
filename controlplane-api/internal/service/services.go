// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// Services groups all mutation services.
type Services struct {
	Team        TeamService
	Application ApplicationService
	Approval    ApprovalService
}

// TeamService defines operations for managing Team resources.
type TeamService interface {
	CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error)
	UpdateTeam(ctx context.Context, input model.UpdateTeamInput) (*model.TeamMutationResult, error)
	RotateTeamToken(ctx context.Context, input model.RotateTeamTokenInput) (*model.TeamMutationResult, error)
}

// ApplicationService defines operations for managing Application resources.
type ApplicationService interface {
	RotateApplicationSecret(ctx context.Context, input model.RotateApplicationSecretInput) (*model.RotateApplicationSecretResult, error)
}

// ApprovalService defines operations for managing Approval and ApprovalRequest resources.
type ApprovalService interface {
	DecideApprovalRequest(ctx context.Context, input model.DecideApprovalRequestInput) (*model.ApprovalMutationResult, error)
	DecideApproval(ctx context.Context, input model.DecideApprovalInput) (*model.ApprovalMutationResult, error)
}
