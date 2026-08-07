// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/Khan/genqlient/graphql"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/client"
	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
	"github.com/telekom/controlplane/organization-server/internal/graphqlinputs"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
)

// HubController defines the business operations for hubs (groups).
type HubController interface {
	Create(ctx context.Context, env string, req api.HubCreateRequest) (*api.HubResponse, []MutationError, error)
	List(ctx context.Context) ([]api.HubResponse, error)
	Get(ctx context.Context, hubName string) (*api.HubResponse, error)
	Update(ctx context.Context, hubName string, req api.HubUpdateRequest) (*api.HubResponse, []MutationError, error)
	Delete(ctx context.Context, hubName string) ([]MutationError, error)
	GetStatus(ctx context.Context, hubName string) (*api.ResourceStatusResponse, error)
}

// TeamController defines the business operations for teams.
type TeamController interface {
	Create(ctx context.Context, env, hubName string, req api.TeamCreateRequest) (*api.TeamResponse, []MutationError, error)
	List(ctx context.Context, hubName string) ([]api.TeamResponse, error)
	Get(ctx context.Context, hubName, teamName string) (*api.TeamResponse, error)
	Update(ctx context.Context, hubName, teamName string, req api.TeamUpdateRequest) (*api.TeamResponse, []MutationError, error)
	Delete(ctx context.Context, hubName, teamName string) ([]MutationError, error)
	GetStatus(ctx context.Context, hubName, teamName string) (*api.ResourceStatusResponse, error)
	RotateToken(ctx context.Context, hubName, teamName string) (string, []MutationError, error)
	GetResources(ctx context.Context, env, hubName, teamName string) ([]client.ResourceRef, error)
}

// MutationError is a common representation for mapping genqlient mutation errors.
type MutationError struct {
	Code    string
	Message string
}

// Controller implements HubController and TeamController.
type Controller struct {
	cpapi graphql.Client
	rover *client.RoverClient
}

// New creates a new Controller with the given upstream clients.
func New(cpapi graphql.Client, rover *client.RoverClient) *Controller {
	return &Controller{
		cpapi: cpapi,
		rover: rover,
	}
}

// --- Hub operations ---

func (ctrl *Controller) Create(ctx context.Context, env string, req *api.HubCreateRequest) (*api.HubResponse, []MutationError, error) {
	desc := req.Description
	resp, err := gql.CreateGroup(ctx, ctrl.cpapi, gql.CreateGroupInput{
		Environment: env,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: &desc,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(resp.CreateGroup.Errors) > 0 {
		return nil, toMutationErrors(resp.CreateGroup.Errors), nil
	}

	g := resp.CreateGroup.Group
	return &api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	}, nil, nil
}

func (ctrl *Controller) List(ctx context.Context) ([]api.HubResponse, error) {
	resp, err := gql.ListGroups(ctx, ctrl.cpapi)
	if err != nil {
		return nil, err
	}

	hubs := make([]api.HubResponse, 0, len(resp.Groups))
	for i := range resp.Groups {
		hubs = append(hubs, mapper.GroupToHubResponse(&resp.Groups[i]))
	}
	return hubs, nil
}

func (ctrl *Controller) Get(ctx context.Context, hubName string) (*api.HubResponse, error) {
	resp, err := gql.GetGroup(ctx, ctrl.cpapi, &graphqlinputs.GroupWhereInput{
		Name: &hubName,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Groups) == 0 {
		return nil, nil
	}

	g := &resp.Groups[0]
	result := mapper.GroupDetailToHubResponse(g)
	return &result, nil
}

func (ctrl *Controller) Update(ctx context.Context, hubName string, req *api.HubUpdateRequest) (*api.HubResponse, []MutationError, error) {
	groupID, err := ctrl.resolveGroupID(ctx, hubName)
	if err != nil {
		return nil, nil, err
	}
	if groupID == "" {
		return nil, nil, nil
	}

	resp, err := gql.UpdateGroup(ctx, ctrl.cpapi, gql.UpdateGroupInput{
		GroupId:     groupID,
		DisplayName: &req.DisplayName,
		Description: &req.Description,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(resp.UpdateGroup.Errors) > 0 {
		return nil, toMutationErrors(resp.UpdateGroup.Errors), nil
	}

	g := resp.UpdateGroup.Group
	return &api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	}, nil, nil
}

func (ctrl *Controller) Delete(ctx context.Context, hubName string) ([]MutationError, error) {
	groupID, err := ctrl.resolveGroupID(ctx, hubName)
	if err != nil {
		return nil, err
	}
	if groupID == "" {
		return []MutationError{{Code: "NOT_FOUND", Message: "Hub not found: " + hubName}}, nil
	}

	resp, err := gql.DeleteGroup(ctx, ctrl.cpapi, gql.DeleteGroupInput{
		GroupId: groupID,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.DeleteGroup.Errors) > 0 {
		return toMutationErrors(resp.DeleteGroup.Errors), nil
	}

	return nil, nil
}

func (ctrl *Controller) GetStatus(_ context.Context, _ string) (*api.ResourceStatusResponse, error) {
	return &api.ResourceStatusResponse{
		OverallStatus:   "done",
		ProcessingState: api.ProcessingStateDone,
		State:           api.StateComplete,
	}, nil
}

// --- Team operations ---

func (ctrl *Controller) CreateTeam(ctx context.Context, env, hubName string, req *api.TeamCreateRequest) (*api.TeamResponse, []MutationError, error) {
	members := make([]gql.MemberInput, 0)
	if req.Members != nil {
		for _, m := range req.Members {
			members = append(members, gql.MemberInput{
				Name:  m.Name,
				Email: m.Email,
			})
		}
	}

	resp, err := gql.CreateTeam(ctx, ctrl.cpapi, gql.CreateTeamInput{
		Environment: env,
		Group:       hubName,
		Name:        req.Name,
		Email:       req.Email,
		Members:     members,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(resp.CreateTeam.Errors) > 0 {
		return nil, toMutationErrors(resp.CreateTeam.Errors), nil
	}

	t := resp.CreateTeam.Team
	teamResp := api.TeamResponse{
		Name:     t.Name,
		Email:    t.Email,
		ClientId: fmt.Sprintf("%s--team-user", t.Name),
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	}
	if t.TeamToken != nil {
		teamResp.TeamToken = *t.TeamToken
	}
	if t.Members != nil {
		mbrs := make([]api.TeamMember, 0, len(t.Members))
		for _, m := range t.Members {
			mbrs = append(mbrs, api.TeamMember{Name: m.Name, Email: m.Email})
		}
		teamResp.Members = mbrs
	}

	return &teamResp, nil, nil
}

func (ctrl *Controller) ListTeams(ctx context.Context, hubName string) ([]api.TeamResponse, error) {
	resp, err := gql.ListTeams(ctx, ctrl.cpapi, &graphqlinputs.TeamWhereInput{
		HasGroupWith: []graphqlinputs.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		return nil, err
	}

	teams := make([]api.TeamResponse, 0, len(resp.Teams.Edges))
	for i := range resp.Teams.Edges {
		team := mapper.TeamToTeamResponse(resp.Teams.Edges[i].Node, !security.IsObfuscated(ctx))
		// CP API exposes the canonical Team CRD name (<group>--<team>); the legacy API exposes the short name.
		team.Name = strings.TrimPrefix(team.Name, hubName+"--")
		teams = append(teams, team)
	}
	return teams, nil
}

func (ctrl *Controller) GetTeam(ctx context.Context, hubName, teamName string) (*api.TeamResponse, error) {
	fullTeamName := hubName + "--" + teamName
	resp, err := gql.GetTeam(ctx, ctrl.cpapi, &graphqlinputs.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []graphqlinputs.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Teams.Edges) == 0 {
		return nil, nil
	}

	result := mapper.GetTeamToTeamResponse(resp.Teams.Edges[0].Node, !security.IsObfuscated(ctx))
	return &result, nil
}

func (ctrl *Controller) UpdateTeam(ctx context.Context, hubName, teamName string, req *api.TeamUpdateRequest) (*api.TeamResponse, []MutationError, error) {
	teamID, err := ctrl.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return nil, nil, err
	}
	if teamID == "" {
		return nil, nil, nil
	}

	resp, err := gql.UpdateTeam(ctx, ctrl.cpapi, gql.UpdateTeamInput{
		TeamId: teamID,
		Email:  &req.Email,
	})
	if err != nil {
		return nil, nil, err
	}

	if len(resp.UpdateTeam.Errors) > 0 {
		return nil, toMutationErrors(resp.UpdateTeam.Errors), nil
	}

	t := resp.UpdateTeam.Team
	teamResp := api.TeamResponse{
		Name:     t.Name,
		Email:    t.Email,
		ClientId: fmt.Sprintf("%s--team-user", t.Name),
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	}
	if t.TeamToken != nil {
		teamResp.TeamToken = *t.TeamToken
	}
	if t.Members != nil {
		mbrs := make([]api.TeamMember, 0, len(t.Members))
		for _, m := range t.Members {
			mbrs = append(mbrs, api.TeamMember{Name: m.Name, Email: m.Email})
		}
		teamResp.Members = mbrs
	}

	return &teamResp, nil, nil
}

func (ctrl *Controller) DeleteTeam(ctx context.Context, hubName, teamName string) ([]MutationError, error) {
	teamID, err := ctrl.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return nil, err
	}
	if teamID == "" {
		return []MutationError{{Code: "NOT_FOUND", Message: "Team not found: " + teamName}}, nil
	}

	resp, err := gql.DeleteTeam(ctx, ctrl.cpapi, gql.DeleteTeamInput{
		TeamId: teamID,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.DeleteTeam.Errors) > 0 {
		return toMutationErrors(resp.DeleteTeam.Errors), nil
	}

	return nil, nil
}

func (ctrl *Controller) GetTeamStatus(ctx context.Context, hubName, teamName string) (*api.ResourceStatusResponse, error) {
	fullTeamName := hubName + "--" + teamName
	resp, err := gql.GetTeam(ctx, ctrl.cpapi, &graphqlinputs.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []graphqlinputs.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Teams.Edges) == 0 {
		return nil, nil
	}

	t := resp.Teams.Edges[0].Node
	status := mapper.TeamStatusToResourceStatus(t.StatusPhase, t.StatusMessage, t.CreatedAt, t.LastModifiedAt)
	return &status, nil
}

func (ctrl *Controller) RotateToken(ctx context.Context, hubName, teamName string) (string, []MutationError, error) {
	teamID, err := ctrl.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return "", nil, err
	}
	if teamID == "" {
		return "", nil, nil
	}

	resp, err := gql.RotateTeamToken(ctx, ctrl.cpapi, teamID)
	if err != nil {
		return "", nil, err
	}

	if len(resp.RotateTeamToken.Errors) > 0 {
		return "", toMutationErrors(resp.RotateTeamToken.Errors), nil
	}

	token := ""
	if resp.RotateTeamToken.Team.TeamToken != nil {
		token = *resp.RotateTeamToken.Team.TeamToken
	}
	return token, nil, nil
}

func (ctrl *Controller) GetResources(ctx context.Context, env, hubName, teamName string) ([]client.ResourceRef, error) {
	resources, err := ctrl.rover.GetResources(ctx, env, hubName, teamName)
	if err != nil {
		return nil, err
	}
	return resources.Items, nil
}

// --- helpers ---

func (ctrl *Controller) resolveGroupID(ctx context.Context, name string) (string, error) {
	resp, err := gql.GetGroup(ctx, ctrl.cpapi, &graphqlinputs.GroupWhereInput{
		Name: &name,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Groups) == 0 {
		return "", nil
	}
	return resp.Groups[0].Id, nil
}

func (ctrl *Controller) resolveTeamID(ctx context.Context, hubName, teamName string) (string, error) {
	fullTeamName := hubName + "--" + teamName
	resp, err := gql.GetTeam(ctx, ctrl.cpapi, &graphqlinputs.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []graphqlinputs.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Teams.Edges) == 0 {
		return "", nil
	}
	return resp.Teams.Edges[0].Node.Id, nil
}

// mutationErrorLike is implemented by all genqlient mutation error types.
type mutationErrorLike interface {
	GetCode() gql.ErrorCode
	GetMessage() string
}

// toMutationErrors converts any slice of genqlient error structs to common MutationErrors.
func toMutationErrors[T any, PT interface {
	*T
	mutationErrorLike
}](errors []T) []MutationError {
	result := make([]MutationError, len(errors))
	for i := range errors {
		p := PT(&errors[i])
		result[i] = MutationError{
			Code:    string(p.GetCode()),
			Message: p.GetMessage(),
		}
	}
	return result
}
