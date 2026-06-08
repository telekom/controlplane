// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// ResourceRef identifies a Kubernetes resource by namespace and name,
// along with authorization context (group and team name).
type ResourceRef struct {
	// Namespace is the Kubernetes namespace of the resource.
	Namespace string
	// Name is the Kubernetes resource name.
	Name string
	// Group is the owning group name (used for authorization).
	Group string
	// TeamName is the owning team name (used for authorization).
	TeamName string
}

// Services groups all mutation services.
type Services struct {
	Team        TeamService
	Application ApplicationService
	Approval    ApprovalService
}

// TeamService defines operations for managing Team resources.
type TeamService interface {
	CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.CreateTeamPayload, error)
	UpdateTeam(ctx context.Context, ref ResourceRef, input model.UpdateTeamInput) (*model.UpdateTeamPayload, error)
	AddTeamMember(ctx context.Context, ref ResourceRef, member model.MemberInput) (*model.AddTeamMemberPayload, error)
	RemoveTeamMember(ctx context.Context, ref ResourceRef, memberEmail string) (*model.RemoveTeamMemberPayload, error)
	RotateTeamToken(ctx context.Context, ref ResourceRef) (*model.RotateTeamTokenPayload, error)
}

// ApplicationService defines operations for managing Application resources.
type ApplicationService interface {
	RotateApplicationSecret(ctx context.Context, ref ResourceRef) (*model.RotateApplicationSecretPayload, error)
}

// ApprovalService defines operations for managing Approval and ApprovalRequest resources.
type ApprovalService interface {
	DecideApprovalRequest(ctx context.Context, ref ResourceRef, input model.DecisionInput) (*model.DecideApprovalRequestPayload, error)
	DecideApproval(ctx context.Context, ref ResourceRef, input model.DecisionInput) (*model.DecideApprovalPayload, error)
}
