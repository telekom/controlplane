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
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
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

// mapApplicationInfo maps an ent Application together with its zone and owning
// team/group to the reduced cross-tenant safe ApplicationInfo view.
// Zone is a fully public type, so it is returned as-is.
func mapApplicationInfo(app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.ApplicationInfo {
	if app == nil {
		return nil
	}
	return &gqlmodel.ApplicationInfo{
		ID:          app.ID,
		Name:        app.Name,
		ExternalIds: app.ExternalIds,
		Zone:        zone,
		OwnerTeam:   mapTeamInfo(team, group),
	}
}

func mapApiExposureInfo(exposure *ent.ApiExposure, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.ApiExposureInfo {
	return &gqlmodel.ApiExposureInfo{
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
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapApiSubscriptionInfo(sub *ent.ApiSubscription, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.ApiSubscriptionInfo {
	var statusPhase *string
	if sub.StatusPhase != nil {
		s := string(*sub.StatusPhase)
		statusPhase = &s
	}
	return &gqlmodel.ApiSubscriptionInfo{
		ID:                   sub.ID,
		BasePath:             sub.BasePath,
		StatusPhase:          statusPhase,
		StatusMessage:        sub.StatusMessage,
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapEventSubscriptionInfo(sub *ent.EventSubscription, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.EventSubscriptionInfo {
	var statusPhase *string
	if sub.StatusPhase != nil {
		s := string(*sub.StatusPhase)
		statusPhase = &s
	}
	return &gqlmodel.EventSubscriptionInfo{
		ID:                   sub.ID,
		EventType:            sub.EventType,
		DeliveryType:         string(sub.DeliveryType),
		StatusPhase:          statusPhase,
		StatusMessage:        sub.StatusMessage,
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapEventExposureInfo(exposure *ent.EventExposure, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.EventExposureInfo {
	return &gqlmodel.EventExposureInfo{
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
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapAgenticExposureInfo(exposure *ent.AgenticExposure, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.AgenticExposureInfo {
	return &gqlmodel.AgenticExposureInfo{
		ID:         exposure.ID,
		BasePath:   exposure.BasePath,
		Visibility: string(exposure.Visibility),
		Variant:    string(exposure.Variant),
		Active:     exposure.Active,
		Traffic:    &exposure.Traffic,
		ApprovalConfig: model.ApprovalConfig{
			Strategy:     exposure.ApprovalConfig.Strategy,
			TrustedTeams: exposure.ApprovalConfig.TrustedTeams,
		},
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapAgenticSubscriptionInfo(sub *ent.AgenticSubscription, app *ent.Application, zone *ent.Zone, team *ent.Team, group *ent.Group) *gqlmodel.AgenticSubscriptionInfo {
	var statusPhase *string
	if sub.StatusPhase != nil {
		s := string(*sub.StatusPhase)
		statusPhase = &s
	}
	return &gqlmodel.AgenticSubscriptionInfo{
		ID:                   sub.ID,
		BasePath:             sub.BasePath,
		StatusPhase:          statusPhase,
		StatusMessage:        sub.StatusMessage,
		OwnerApplicationName: app.Name,
		OwnerTeam:            mapTeamInfo(team, group),
		OwnerApplication:     mapApplicationInfo(app, zone, team, group),
	}
}

func mapListenerInfo(listener *ent.Listener) (*gqlmodel.ListenerInfo, error) {
	subscription, err := listener.Edges.SubscriptionOrErr()
	if err != nil {
		return nil, fmt.Errorf("loading subscription edge for listener %d: %w", listener.ID, err)
	}
	exposure, err := listener.Edges.ExposureOrErr()
	if err != nil {
		return nil, fmt.Errorf("loading exposure edge for listener %d: %w", listener.ID, err)
	}
	application, applicationZone, applicationTeam, applicationGroup, err := loadedOwnerChain(listener.Edges.Application, listener.ID, "listener")
	if err != nil {
		return nil, err
	}
	consumer, consumerZone, consumerTeam, consumerGroup, err := loadedOwnerChain(subscription.Edges.Owner, listener.ID, "consumer")
	if err != nil {
		return nil, err
	}
	provider, providerZone, providerTeam, providerGroup, err := loadedOwnerChain(exposure.Edges.Owner, listener.ID, "provider")
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
	applicationInfo := mapApplicationInfo(application, applicationZone, applicationTeam, applicationGroup)
	return &gqlmodel.ListenerInfo{
		ID:               listener.ID,
		ResourceName:     *apiDefinition.Name,
		Approved:         approved,
		Application:      applicationInfo,
		Consumer:         mapApplicationInfo(consumer, consumerZone, consumerTeam, consumerGroup),
		Provider:         mapApplicationInfo(provider, providerZone, providerTeam, providerGroup),
		OwnerApplication: applicationInfo,
	}, nil
}

func withListenerInfo(query *ent.ListenerQuery) *ent.ListenerQuery {
	return query.
		WithApplication(func(q *ent.ApplicationQuery) {
			q.WithZone()
			q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
		}).
		WithSubscription(func(q *ent.ApiSubscriptionQuery) {
			q.WithOwner(func(q *ent.ApplicationQuery) {
				q.WithZone()
				q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
			})
		}).
		WithExposure(func(q *ent.ApiExposureQuery) {
			q.WithAPI()
			q.WithOwner(func(q *ent.ApplicationQuery) {
				q.WithZone()
				q.WithOwnerTeam(func(q *ent.TeamQuery) { q.WithGroup() })
			})
		}).
		WithProviderApproval().
		WithConsumerApproval()
}

func loadListenerInfo(ctx context.Context, client *ent.Client, listener *ent.Listener) (*gqlmodel.ListenerInfo, error) {
	loaded, err := withListenerInfo(client.Listener.Query()).Where(entlistener.IDEQ(listener.ID)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading listener %d: %w", listener.ID, err)
	}
	return mapListenerInfo(loaded)
}

func mapListenerInfos(listeners []*ent.Listener) ([]*gqlmodel.ListenerInfo, error) {
	result := make([]*gqlmodel.ListenerInfo, len(listeners))
	for i, listener := range listeners {
		info, err := mapListenerInfo(listener)
		if err != nil {
			return nil, err
		}
		result[i] = info
	}
	return result, nil
}

func loadedOwnerChain(app *ent.Application, listenerID int, role string) (*ent.Application, *ent.Zone, *ent.Team, *ent.Group, error) {
	if app == nil {
		return nil, nil, nil, nil, fmt.Errorf("loading %s application for listener %d: edge not loaded", role, listenerID)
	}
	zone, err := app.Edges.ZoneOrErr()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("loading %s zone for listener %d: %w", role, listenerID, err)
	}
	team, err := app.Edges.OwnerTeamOrErr()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("loading %s team for listener %d: %w", role, listenerID, err)
	}
	group, err := team.Edges.GroupOrErr()
	if err != nil && !ent.IsNotFound(err) && !ent.IsNotLoaded(err) {
		return nil, nil, nil, nil, fmt.Errorf("loading group for %s team %d: %w", role, team.ID, err)
	}
	if err != nil {
		group = nil
	}
	return app, zone, team, group, nil
}

// loadOwnerChain traverses subscription → owner application → zone/team → group.
// Used by both API and event subscription info loaders.
func loadOwnerChain(ctx context.Context, ownerQuery interface {
	Only(context.Context) (*ent.Application, error)
},
) (*ent.Application, *ent.Zone, *ent.Team, *ent.Group, error) {
	app, err := ownerQuery.Only(ctx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("loading owner application: %w", err)
	}
	// The zone edge is required in ent, so a not-found is a real error here.
	zone, err := app.QueryZone().Only(ctx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("loading zone for application %d: %w", app.ID, err)
	}
	team, err := app.QueryOwnerTeam().Only(ctx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("loading owner team for application %d: %w", app.ID, err)
	}
	group, err := team.QueryGroup().Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, nil, nil, nil, fmt.Errorf("loading group for team %d: %w", team.ID, err)
	}
	if ent.IsNotFound(err) {
		group = nil
	}
	return app, zone, team, group, nil
}

// loadApiSubscriptionInfo loads the full owner chain for an API subscription and maps it to ApiSubscriptionInfo.
func loadApiSubscriptionInfo(ctx context.Context, sub *ent.ApiSubscription) (*gqlmodel.ApiSubscriptionInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, sub.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("api subscription %d: %w", sub.ID, err)
	}
	return mapApiSubscriptionInfo(sub, app, zone, team, group), nil
}

// loadEventSubscriptionInfo loads the full owner chain for an event subscription and maps it to EventSubscriptionInfo.
func loadEventSubscriptionInfo(ctx context.Context, sub *ent.EventSubscription) (*gqlmodel.EventSubscriptionInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, sub.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("event subscription %d: %w", sub.ID, err)
	}
	return mapEventSubscriptionInfo(sub, app, zone, team, group), nil
}

// loadApiExposureInfo loads the full owner chain for an API exposure and maps it to ApiExposureInfo.
func loadApiExposureInfo(ctx context.Context, exposure *ent.ApiExposure) (*gqlmodel.ApiExposureInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, exposure.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("api exposure %d: %w", exposure.ID, err)
	}
	return mapApiExposureInfo(exposure, app, zone, team, group), nil
}

// loadEventExposureInfo loads the full owner chain for an event exposure and maps it to EventExposureInfo.
func loadEventExposureInfo(ctx context.Context, exposure *ent.EventExposure) (*gqlmodel.EventExposureInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, exposure.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("event exposure %d: %w", exposure.ID, err)
	}
	return mapEventExposureInfo(exposure, app, zone, team, group), nil
}

// loadAgenticSubscriptionInfo loads the full owner chain for an agentic subscription and maps it to AgenticSubscriptionInfo.
func loadAgenticSubscriptionInfo(ctx context.Context, sub *ent.AgenticSubscription) (*gqlmodel.AgenticSubscriptionInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, sub.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("agentic subscription %d: %w", sub.ID, err)
	}
	return mapAgenticSubscriptionInfo(sub, app, zone, team, group), nil
}

// loadAgenticExposureInfo loads the full owner chain for an agentic exposure and maps it to AgenticExposureInfo.
func loadAgenticExposureInfo(ctx context.Context, exposure *ent.AgenticExposure) (*gqlmodel.AgenticExposureInfo, error) {
	app, zone, team, group, err := loadOwnerChain(ctx, exposure.QueryOwner())
	if err != nil {
		return nil, fmt.Errorf("agentic exposure %d: %w", exposure.ID, err)
	}
	return mapAgenticExposureInfo(exposure, app, zone, team, group), nil
}

func loadApplicationInfo(ctx context.Context, app *ent.Application) (*gqlmodel.ApplicationInfo, error) {
	zone, err := app.QueryZone().Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading zone for application %d: %w", app.ID, err)
	}
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
	return mapApplicationInfo(app, zone, team, group), nil
}
