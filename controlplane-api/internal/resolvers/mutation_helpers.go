// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
)

// mutationsDisabledError returns a MutationError indicating that mutations
// are disabled (the corresponding service is nil).
func mutationsDisabledError() model.MutationError {
	return model.MutationError{
		Code:    model.ErrorCodePreconditionFailed,
		Message: "mutations are disabled",
	}
}

// notFoundMutationError returns a MutationError for a not-found resource.
func notFoundMutationError(msg string) model.MutationError {
	return model.MutationError{
		Code:    model.ErrorCodeNotFound,
		Message: msg,
	}
}

// resolveTeamRef looks up a Team by its ent ID and builds a ResourceRef
// that the service layer needs for Kubernetes operations.
func (r *mutationResolver) resolveTeamRef(ctx context.Context, teamID int) (*ent.Team, service.ResourceRef, error) {
	t, err := r.client.Team.Get(ctx, teamID)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting team: %w", err)
	}

	group, err := t.QueryGroup().Only(ctx)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting team group: %w", err)
	}

	ref := service.ResourceRef{
		Namespace: t.Namespace,
		Name:      t.Name,
		Group:     group.Name,
		TeamName:  t.Name,
	}

	return t, ref, nil
}

// resolveApplicationRef looks up an Application by its ent ID and builds a ResourceRef.
func (r *mutationResolver) resolveApplicationRef(ctx context.Context, applicationID int) (*ent.Application, service.ResourceRef, error) {
	app, err := r.client.Application.Get(ctx, applicationID)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting application: %w", err)
	}

	team, err := app.QueryOwnerTeam().Only(ctx)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting application owner team: %w", err)
	}

	group, err := team.QueryGroup().Only(ctx)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting application owner group: %w", err)
	}

	ref := service.ResourceRef{
		Namespace: app.Namespace,
		Name:      app.Name,
		Group:     group.Name,
		TeamName:  team.Name,
	}

	return app, ref, nil
}

// resolveApprovalRequestRef looks up an ApprovalRequest by its ent ID and builds a ResourceRef.
func (r *mutationResolver) resolveApprovalRequestRef(ctx context.Context, approvalRequestID int) (*ent.ApprovalRequest, service.ResourceRef, error) {
	ar, err := r.client.ApprovalRequest.Get(ctx, approvalRequestID)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting approval request: %w", err)
	}

	ref := service.ResourceRef{
		Namespace: ar.Namespace,
		Name:      ar.Name,
		TeamName:  ar.DeciderTeamName,
	}

	return ar, ref, nil
}

// resolveApprovalRef looks up an Approval by its ent ID and builds a ResourceRef.
func (r *mutationResolver) resolveApprovalRef(ctx context.Context, approvalID int) (*ent.Approval, service.ResourceRef, error) {
	a, err := r.client.Approval.Get(ctx, approvalID)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting approval: %w", err)
	}

	ref := service.ResourceRef{
		Namespace: a.Namespace,
		Name:      a.Name,
		TeamName:  a.DeciderTeamName,
	}

	return a, ref, nil
}

// resolveGroupRef looks up a Group by its ent ID and builds a ResourceRef.
func (r *mutationResolver) resolveGroupRef(ctx context.Context, groupID int) (*ent.Group, service.ResourceRef, error) {
	g, err := r.client.Group.Get(ctx, groupID)
	if err != nil {
		return nil, service.ResourceRef{}, fmt.Errorf("getting group: %w", err)
	}

	ref := service.ResourceRef{
		Namespace: g.Namespace,
		Name:      g.Name,
		Group:     g.Name,
	}

	return g, ref, nil
}
