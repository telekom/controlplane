// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
)

func mapTeamInfo(team *ent.Team, group *ent.Group) *model.TeamInfo {
	groupName := ""
	if group != nil {
		groupName = group.Name
	}
	var email *string
	if team.Email != "" {
		email = &team.Email
	}
	return &model.TeamInfo{
		ID:        team.ID,
		Name:      team.Name,
		GroupName: groupName,
		Email:     email,
	}
}

func mapApiExposureInfo(exposure *ent.ApiExposure, app *ent.Application, team *ent.Team, group *ent.Group) *model.ApiExposureInfo {
	return &model.ApiExposureInfo{
		ID:         exposure.ID,
		BasePath:   exposure.BasePath,
		Visibility: string(exposure.Visibility),
		Active:     exposure.Active,
		ApiVersion: exposure.APIVersion,
		Features:   exposure.Features,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     exposure.ApprovalConfig.Strategy,
			TrustedTeams: exposure.ApprovalConfig.TrustedTeams,
		},
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
	}
}

func mapApiSubscriptionInfo(sub *ent.ApiSubscription, app *ent.Application, team *ent.Team, group *ent.Group) *model.ApiSubscriptionInfo {
	var statusPhase *string
	if sub.StatusPhase != nil {
		s := string(*sub.StatusPhase)
		statusPhase = &s
	}
	return &model.ApiSubscriptionInfo{
		ID:                   sub.ID,
		BasePath:             sub.BasePath,
		StatusPhase:          statusPhase,
		StatusMessage:        sub.StatusMessage,
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
	}
}

func mapEventSubscriptionInfo(sub *ent.EventSubscription, app *ent.Application, team *ent.Team, group *ent.Group) *model.EventSubscriptionInfo {
	var statusPhase *string
	if sub.StatusPhase != nil {
		s := string(*sub.StatusPhase)
		statusPhase = &s
	}
	return &model.EventSubscriptionInfo{
		ID:                   sub.ID,
		EventType:            sub.EventType,
		DeliveryType:         string(sub.DeliveryType),
		StatusPhase:          statusPhase,
		StatusMessage:        sub.StatusMessage,
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
	}
}

// loadOwnerChain traverses subscription → owner application → team → group.
// Used by both API and event subscription info loaders.
func loadOwnerChain(ctx context.Context, ownerQuery interface {
	Only(context.Context) (*ent.Application, error)
}) (*ent.Application, *ent.Team, *ent.Group, error) {
	app, err := ownerQuery.Only(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading owner application: %w", err)
	}
	team, err := app.QueryOwnerTeam().Only(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading owner team for application %d: %w", app.ID, err)
	}
	group, err := team.QueryGroup().Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, nil, nil, fmt.Errorf("loading group for team %d: %w", team.ID, err)
	}
	if ent.IsNotFound(err) {
		group = nil
	}
	return app, team, group, nil
}

// loadApiSubscriptionInfo loads the full owner chain for an API subscription and maps it to ApiSubscriptionInfo.
func loadApiSubscriptionInfo(ctx context.Context, sub *ent.ApiSubscription) (*model.ApiSubscriptionInfo, error) {
	app, team, group, err := loadOwnerChain(ctx, sub.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("api subscription %d: %w", sub.ID, err)
	}
	return mapApiSubscriptionInfo(sub, app, team, group), nil
}

// loadEventSubscriptionInfo loads the full owner chain for an event subscription and maps it to EventSubscriptionInfo.
func loadEventSubscriptionInfo(ctx context.Context, sub *ent.EventSubscription) (*model.EventSubscriptionInfo, error) {
	app, team, group, err := loadOwnerChain(ctx, sub.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("event subscription %d: %w", sub.ID, err)
	}
	return mapEventSubscriptionInfo(sub, app, team, group), nil
}
