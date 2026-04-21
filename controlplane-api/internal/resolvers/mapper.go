// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
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
