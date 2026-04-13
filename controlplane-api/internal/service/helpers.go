// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/vektah/gqlparser/v2/gqlerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// teamResourceName returns the K8s resource name for a team: <group>--<team>.
func teamResourceName(group, team string) string {
	return group + "--" + team
}

// mapK8sError converts a Kubernetes API error to a GraphQL error with an appropriate error code.
func mapK8sError(err error) *gqlerror.Error {
	if err == nil {
		return nil
	}

	switch {
	case apierrors.IsNotFound(err):
		return &gqlerror.Error{
			Message:    "resource not found",
			Extensions: map[string]interface{}{"code": "NOT_FOUND"},
		}
	case apierrors.IsAlreadyExists(err):
		return &gqlerror.Error{
			Message:    "resource already exists",
			Extensions: map[string]interface{}{"code": "ALREADY_EXISTS"},
		}
	case apierrors.IsConflict(err):
		return &gqlerror.Error{
			Message:    "conflict: resource was modified concurrently",
			Extensions: map[string]interface{}{"code": "CONFLICT"},
		}
	case apierrors.IsForbidden(err):
		return &gqlerror.Error{
			Message:    "forbidden by Kubernetes RBAC",
			Extensions: map[string]interface{}{"code": "FORBIDDEN"},
		}
	case apierrors.IsInvalid(err):
		return &gqlerror.Error{
			Message:    fmt.Sprintf("validation failed: %s", err),
			Extensions: map[string]interface{}{"code": "VALIDATION_ERROR"},
		}
	default:
		return &gqlerror.Error{
			Message:    fmt.Sprintf("internal error: %s", err),
			Extensions: map[string]interface{}{"code": "INTERNAL"},
		}
	}
}
