// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/api"
	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
)

// GroupToHubResponse maps a genqlient Group to the legacy HubResponse.
func GroupToHubResponse(g *gql.ListGroupsGroupsGroup) api.HubResponse {
	return api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
	}
}

// GroupDetailToHubResponse maps a single group (from GetGroup) to HubResponse.
func GroupDetailToHubResponse(g *gql.GetGroupGroupsGroup) api.HubResponse {
	return api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
	}
}

// TeamToTeamResponse maps a genqlient Team node to the legacy TeamResponse.
func TeamToTeamResponse(t *gql.ListTeamsTeamsTeamConnectionEdgesTeamEdgeNodeTeam) api.TeamResponse {
	groupName := ""
	if t.Group != nil {
		groupName = t.Group.Name
	}

	members := make([]api.TeamMember, 0, len(t.Members))
	for _, m := range t.Members {
		members = append(members, api.TeamMember{
			Name:  m.Name,
			Email: m.Email,
		})
	}

	clientID := fmt.Sprintf("%s--%s--team-user", groupName, t.Name)

	resp := api.TeamResponse{
		Name:     t.Name,
		Email:    t.Email,
		Members:  members,
		ClientId: clientID,
		Status:   mapStatusPhase(t.StatusPhase, t.StatusMessage),
	}
	if t.TeamToken != nil {
		resp.TeamToken = *t.TeamToken
	}

	return resp
}

// GetTeamToTeamResponse maps a team from GetTeam query.
func GetTeamToTeamResponse(t *gql.GetTeamTeamsTeamConnectionEdgesTeamEdgeNodeTeam) api.TeamResponse {
	groupName := ""
	if t.Group != nil {
		groupName = t.Group.Name
	}

	members := make([]api.TeamMember, 0, len(t.Members))
	for _, m := range t.Members {
		members = append(members, api.TeamMember{
			Name:  m.Name,
			Email: m.Email,
		})
	}

	clientID := fmt.Sprintf("%s--%s--team-user", groupName, t.Name)

	resp := api.TeamResponse{
		Name:     t.Name,
		Email:    t.Email,
		Members:  members,
		ClientId: clientID,
		Status:   mapStatusPhase(t.StatusPhase, t.StatusMessage),
	}
	if t.TeamToken != nil {
		resp.TeamToken = *t.TeamToken
	}

	return resp
}

// mapStatusPhase translates CP's statusPhase to the legacy Status shape.
func mapStatusPhase(phase *gql.TeamStatusPhase, message *string) api.Status {
	if phase == nil {
		return api.Status{
			ProcessingState: "none",
			State:           "none",
		}
	}

	switch string(*phase) {
	case "READY":
		return api.Status{
			ProcessingState: "done",
			State:           "complete",
		}
	case "PENDING":
		return api.Status{
			ProcessingState: "pending",
			State:           "none",
		}
	case "ERROR":
		s := api.Status{
			ProcessingState: "failed",
			State:           "blocked",
		}
		if message != nil && *message != "" {
			s.Errors = []api.StateInfo{{Message: *message}}
		}
		return s
	default: // UNKNOWN or unexpected
		return api.Status{
			ProcessingState: "none",
			State:           "none",
		}
	}
}

// TeamStatusToResourceStatus maps a team's status to the legacy ResourceStatusResponse.
func TeamStatusToResourceStatus(phase *gql.TeamStatusPhase, message *string, createdAt, lastModifiedAt time.Time) api.ResourceStatusResponse {
	resp := api.ResourceStatusResponse{
		State:           api.StateNone,
		ProcessingState: api.ProcessingStateNone,
		OverallStatus:   "none",
		CreatedAt:       createdAt,
		ProcessedAt:     lastModifiedAt,
	}

	if phase == nil {
		return resp
	}

	switch string(*phase) {
	case "READY":
		resp.State = api.StateComplete
		resp.ProcessingState = api.ProcessingStateDone
		resp.OverallStatus = "done"
	case "PENDING":
		resp.State = api.StateNone
		resp.ProcessingState = api.ProcessingStatePending
		resp.OverallStatus = "pending"
	case "ERROR":
		resp.State = api.StateBlocked
		resp.ProcessingState = api.ProcessingStateFailed
		resp.OverallStatus = "failed"
		if message != nil && *message != "" {
			resp.Errors = []api.Problem{{Message: *message}}
		}
	}

	return resp
}

type PaginationParams struct {
	Offset int
	Limit  int
}

// ParsePagination extracts offset/limit from Fiber context.
func ParsePagination(c *fiber.Ctx) PaginationParams {
	offset := c.QueryInt("offset", 0)
	limit := c.QueryInt("limit", 20)
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}
	return PaginationParams{Offset: offset, Limit: limit}
}

// BuildPaginatedResponse builds the standard paginated response with _links and paging.
func BuildPaginatedResponse(c *fiber.Ctx, items any, total int, p PaginationParams) fiber.Map {
	page := (p.Offset / p.Limit) + 1
	lastPage := (total + p.Limit - 1) / p.Limit
	if lastPage < 1 {
		lastPage = 1
	}

	basePath := c.Path()
	return fiber.Map{
		"_links": fiber.Map{
			"first": basePath + "?offset=0&limit=" + strconv.Itoa(p.Limit),
			"last":  basePath + "?offset=" + strconv.Itoa((lastPage-1)*p.Limit) + "&limit=" + strconv.Itoa(p.Limit),
			"self":  basePath + "?offset=" + strconv.Itoa(p.Offset) + "&limit=" + strconv.Itoa(p.Limit),
		},
		"items": items,
		"paging": fiber.Map{
			"last_page": lastPage,
			"page":      page,
			"total":     total,
		},
	}
}
