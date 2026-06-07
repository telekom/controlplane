// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// authorizeCreateTeam checks that the viewer is allowed to create a team in the given group.
// Allowed: Admin, or Group viewer matching the target group.
func authorizeCreateTeam(ctx context.Context, targetGroup string) error {
	v := viewer.FromContext(ctx)
	if v == nil {
		return fmt.Errorf("unauthorized: no viewer in context")
	}
	if v.Admin {
		return nil
	}
	if v.Group != "" && v.Group == targetGroup {
		return nil
	}
	return fmt.Errorf("forbidden: insufficient permissions to create team in group %q", targetGroup)
}

// authorizeUpdateTeam checks that the viewer is allowed to update a team.
// Allowed: Admin, Group viewer matching the target group, or Team viewer matching the target team.
func authorizeUpdateTeam(ctx context.Context, targetGroup, targetTeam string) error {
	v := viewer.FromContext(ctx)
	if v == nil {
		return fmt.Errorf("unauthorized: no viewer in context")
	}
	if v.Admin {
		return nil
	}
	if v.Group != "" && v.Group == targetGroup {
		return nil
	}
	if v.HasTeam(targetTeam) {
		return nil
	}
	return fmt.Errorf("forbidden: insufficient permissions to update team %q", targetTeam)
}

// authorizeApplicationAction checks that the viewer is allowed to act on an application.
// Allowed: Admin, or Team viewer matching the application's owning team.
func authorizeApplicationAction(ctx context.Context, appTeam string) error {
	v := viewer.FromContext(ctx)
	if v == nil {
		return fmt.Errorf("unauthorized: no viewer in context")
	}
	if v.Admin {
		return nil
	}
	if v.HasTeam(appTeam) {
		return nil
	}
	return fmt.Errorf("forbidden: insufficient permissions for application owned by team %q", appTeam)
}

// authorizeApprovalAction checks that the viewer is allowed to decide on an approval.
// Allowed: Admin, or Team viewer matching the decider team.
func authorizeApprovalAction(ctx context.Context, deciderTeam string) error {
	v := viewer.FromContext(ctx)
	if v == nil {
		return fmt.Errorf("unauthorized: no viewer in context")
	}
	if v.Admin {
		return nil
	}
	if v.HasTeam(deciderTeam) {
		return nil
	}
	return fmt.Errorf("forbidden: insufficient permissions — only the decider team %q can decide on this approval", deciderTeam)
}
