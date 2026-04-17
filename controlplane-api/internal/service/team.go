// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
)

// TeamService defines operations for managing Team resources.
type TeamService interface {
	CreateTeam(ctx context.Context, input model.CreateTeamInput) (*model.TeamMutationResult, error)
	UpdateTeam(ctx context.Context, input model.UpdateTeamInput) (*model.TeamMutationResult, error)
	RotateTeamToken(ctx context.Context, input model.RotateTeamTokenInput) (*model.TeamMutationResult, error)
}
