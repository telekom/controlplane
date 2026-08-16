// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entgroup "github.com/telekom/controlplane/controlplane-api/ent/group"
	entteam "github.com/telekom/controlplane/controlplane-api/ent/team"
)

// TeamChecker checks whether a group has any teams.
type TeamChecker interface {
	// HasTeams returns true if the given group has at least one team.
	HasTeams(ctx context.Context, groupName string) (bool, error)
}

// entTeamChecker implements TeamChecker by querying the projected ent database.
type entTeamChecker struct {
	client *ent.Client
}

// NewEntTeamChecker creates a TeamChecker that queries the projected database.
func NewEntTeamChecker(client *ent.Client) TeamChecker {
	return &entTeamChecker{client: client}
}

func (t *entTeamChecker) HasTeams(ctx context.Context, groupName string) (bool, error) {
	exists, err := t.client.Team.Query().
		Where(entteam.HasGroupWith(entgroup.Name(groupName))).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("checking teams for group %q: %w", groupName, err)
	}
	return exists, nil
}

// noopTeamChecker always returns false (no teams). Used in tests or when
// the check is not needed.
type noopTeamChecker struct{}

// NewNoopTeamChecker creates a TeamChecker that always reports no teams.
func NewNoopTeamChecker() TeamChecker {
	return &noopTeamChecker{}
}

func (n *noopTeamChecker) HasTeams(_ context.Context, _ string) (bool, error) {
	return false, nil
}
