// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers

import (
	"context"
	"fmt"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/approval"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
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
		ID:          team.ID,
		Name:        team.Name,
		GroupName:   groupName,
		Email:       email,
		DisplayName: team.DisplayName,
		Description: team.Description,
	}
}

func mapApplicationInfo(app *ent.Application, team *ent.Team, group *ent.Group) *model.ApplicationInfo {
	return &model.ApplicationInfo{
		ID:        app.ID,
		Name:      app.Name,
		OwnerTeam: mapTeamInfo(team, group),
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
		Traffic:    &exposure.Traffic,
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

func mapEventExposureInfo(exposure *ent.EventExposure, app *ent.Application, team *ent.Team, group *ent.Group) *model.EventExposureInfo {
	return &model.EventExposureInfo{
		ID:         exposure.ID,
		EventType:  exposure.EventType,
		Visibility: string(exposure.Visibility),
		Active:     exposure.Active,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     exposure.ApprovalConfig.Strategy,
			TrustedTeams: exposure.ApprovalConfig.TrustedTeams,
		},
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
	}
}

func mapListenerInfo(listener *ent.Listener) (*model.ListenerInfo, error) {
	subscription, err := listener.Edges.SubscriptionOrErr()
	if err != nil {
		return nil, fmt.Errorf("loading subscription edge for listener %d: %w", listener.ID, err)
	}
	exposure, err := listener.Edges.ExposureOrErr()
	if err != nil {
		return nil, fmt.Errorf("loading exposure edge for listener %d: %w", listener.ID, err)
	}
	application, applicationTeam, applicationGroup, err := loadedOwnerChain(listener.Edges.Application, listener.ID, "listener")
	if err != nil {
		return nil, err
	}
	consumer, consumerTeam, consumerGroup, err := loadedOwnerChain(subscription.Edges.Owner, listener.ID, "consumer")
	if err != nil {
		return nil, err
	}
	provider, providerTeam, providerGroup, err := loadedOwnerChain(exposure.Edges.Owner, listener.ID, "provider")
	if err != nil {
		return nil, err
	}
	apiDefinition, err := exposure.Edges.APIOrErr()
	if err != nil {
		return nil, fmt.Errorf("loading api edge for listener %d: %w", listener.ID, err)
	}
	if apiDefinition.Name == nil || *apiDefinition.Name == "" {
		return nil, fmt.Errorf("listener %d references api %d without a projected kubernetes name", listener.ID, apiDefinition.ID)
	}

	approved := listener.Edges.ProviderApproval != nil && listener.Edges.ProviderApproval.State == approval.StateGranted &&
		listener.Edges.ConsumerApproval != nil && listener.Edges.ConsumerApproval.State == approval.StateGranted
	return &model.ListenerInfo{
		ID:           listener.ID,
		ResourceName: *apiDefinition.Name,
		Approved:     approved,
		Application:  mapApplicationInfo(application, applicationTeam, applicationGroup),
		Consumer:     mapApplicationInfo(consumer, consumerTeam, consumerGroup),
		Provider:     mapApplicationInfo(provider, providerTeam, providerGroup),
	}, nil
}

func withListenerInfo(query *ent.ListenerQuery) *ent.ListenerQuery {
	return query.
		WithApplication(func(q *ent.ApplicationQuery) {
			q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
		}).
		WithSubscription(func(q *ent.ApiSubscriptionQuery) {
			q.WithOwner(func(q *ent.ApplicationQuery) {
				q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
			})
		}).
		WithExposure(func(q *ent.ApiExposureQuery) {
			q.WithAPI()
			q.WithOwner(func(q *ent.ApplicationQuery) {
				q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
			})
		}).
		WithProviderApproval().
		WithConsumerApproval()
}

func loadListenerInfo(ctx context.Context, client *ent.Client, listener *ent.Listener) (*model.ListenerInfo, error) {
	loaded, err := withListenerInfo(client.Listener.Query()).Where(entlistener.IDEQ(listener.ID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading listener %d: %w", listener.ID, err)
	}
	return mapListenerInfo(loaded)
}

func mapListenerInfos(listeners []*ent.Listener) ([]*model.ListenerInfo, error) {
	result := make([]*model.ListenerInfo, len(listeners))
	for i, listener := range listeners {
		info, err := mapListenerInfo(listener)
		if err != nil {
			return nil, err
		}
		result[i] = info
	}
	return result, nil
}

func loadedOwnerChain(app *ent.Application, listenerID int, role string) (*ent.Application, *ent.Team, *ent.Group, error) {
	if app == nil {
		return nil, nil, nil, fmt.Errorf("loading %s application for listener %d: edge not loaded", role, listenerID)
	}
	team, err := app.Edges.OwnerTeamOrErr()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading %s team for listener %d: %w", role, listenerID, err)
	}
	group, err := team.Edges.GroupOrErr()
	if err != nil && !ent.IsNotFound(err) && !ent.IsNotLoaded(err) {
		return nil, nil, nil, fmt.Errorf("loading group for %s team %d: %w", role, team.ID, err)
	}
	if err != nil {
		group = nil
	}
	return app, team, group, nil
}

// loadOwnerChain traverses subscription → owner application → team → group.
// Used by both API and event subscription info loaders.
func loadOwnerChain(ctx context.Context, ownerQuery interface {
	Only(context.Context) (*ent.Application, error)
},
) (*ent.Application, *ent.Team, *ent.Group, error) {
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

// loadApiExposureInfo loads the full owner chain for an API exposure and maps it to ApiExposureInfo.
func loadApiExposureInfo(ctx context.Context, exposure *ent.ApiExposure) (*model.ApiExposureInfo, error) {
	app, team, group, err := loadOwnerChain(ctx, exposure.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("api exposure %d: %w", exposure.ID, err)
	}
	return mapApiExposureInfo(exposure, app, team, group), nil
}

// loadEventExposureInfo loads the full owner chain for an event exposure and maps it to EventExposureInfo.
func loadEventExposureInfo(ctx context.Context, exposure *ent.EventExposure) (*model.EventExposureInfo, error) {
	app, team, group, err := loadOwnerChain(ctx, exposure.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("event exposure %d: %w", exposure.ID, err)
	}
	return mapEventExposureInfo(exposure, app, team, group), nil
}

func loadApplicationInfo(ctx context.Context, app *ent.Application) (*model.ApplicationInfo, error) {
	team, err := app.QueryOwnerTeam().Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading owner team for application %d: %w", app.ID, err)
	}
	group, err := team.QueryGroup().Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, fmt.Errorf("loading group for team %d: %w", team.ID, err)
	}
	if ent.IsNotFound(err) {
		group = nil
	}
	return mapApplicationInfo(app, team, group), nil
}
