// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/api"
	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
	mw "github.com/telekom/controlplane/organization-server/internal/middleware"
)

// CreateTeam handles POST /hubs/:hub/teams.
func (h *Handler) CreateTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	var req api.TeamCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: float32(400),
			Detail: "Invalid request body",
		})
	}

	members := make([]gql.MemberInput, 0)
	if req.Members != nil {
		for _, m := range req.Members {
			members = append(members, gql.MemberInput{
				Name:  m.Name,
				Email: m.Email,
			})
		}
	}

	ctx := h.contextWithIdentity(c)
	id := mw.ConsumerIdentityFromContext(c)
	resp, err := gql.CreateTeam(ctx, h.cpapi, gql.CreateTeamInput{
		Environment: id.Environment,
		Group:       hubName,
		Name:        req.Name,
		Email:       req.Email,
		Members:     members,
	})
	if err != nil {
		h.log.Error(err, "Failed to create team", "hub", hubName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to create team",
		})
	}

	if len(resp.CreateTeam.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.CreateTeam.Errors))
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
		members := make([]api.TeamMember, 0, len(t.Members))
		for _, m := range t.Members {
			members = append(members, api.TeamMember{Name: m.Name, Email: m.Email})
		}
		teamResp.Members = members
	}

	return c.Status(fiber.StatusAccepted).JSON(teamResp)
}

// ListTeams handles GET /hubs/:hub/teams.
func (h *Handler) ListTeams(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	ctx := h.contextWithIdentity(c)
	resp, err := gql.ListTeams(ctx, h.cpapi, &gql.TeamWhereInput{
		HasGroupWith: []gql.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		h.log.Error(err, "Failed to list teams", "hub", hubName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to list teams",
		})
	}

	teams := make([]api.TeamResponse, 0, len(resp.Teams.Edges))
	for i := range resp.Teams.Edges {
		teams = append(teams, mapper.TeamToTeamResponse(resp.Teams.Edges[i].Node))
	}

	p := mapper.ParsePagination(c)
	total := len(teams)

	start := p.Offset
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	paged := teams[start:end]

	c.Set("X-Total-Count", intToStr(total))
	c.Set("X-Result-Count", intToStr(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}

// GetTeam handles GET /hubs/:hub/teams/:team.
func (h *Handler) GetTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	fullTeamName := hubName + "--" + teamName

	ctx := h.contextWithIdentity(c)
	resp, err := gql.GetTeam(ctx, h.cpapi, &gql.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []gql.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		h.log.Error(err, "Failed to get team", "hub", hubName, "team", teamName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to get team",
		})
	}

	if len(resp.Teams.Edges) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Team not found: " + teamName,
		})
	}

	return c.JSON(mapper.GetTeamToTeamResponse(resp.Teams.Edges[0].Node))
}

// UpdateTeam handles PUT /hubs/:hub/teams/:team.
func (h *Handler) UpdateTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")

	var req api.TeamUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: float32(400),
			Detail: "Invalid request body",
		})
	}

	ctx := h.contextWithIdentity(c)

	// Resolve team ID.
	teamID, err := h.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return err
	}
	if teamID == "" {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Team not found: " + teamName,
		})
	}

	resp, err := gql.UpdateTeam(ctx, h.cpapi, gql.UpdateTeamInput{
		TeamId: teamID,
		Email:  &req.Email,
	})
	if err != nil {
		h.log.Error(err, "Failed to update team", "hub", hubName, "team", teamName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to update team",
		})
	}

	if len(resp.UpdateTeam.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.UpdateTeam.Errors))
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

	return c.Status(fiber.StatusAccepted).JSON(teamResp)
}

// DeleteTeam handles DELETE /hubs/:hub/teams/:team.
func (h *Handler) DeleteTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")

	ctx := h.contextWithIdentity(c)

	teamID, err := h.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return err
	}
	if teamID == "" {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Team not found: " + teamName,
		})
	}

	resp, err := gql.DeleteTeam(ctx, h.cpapi, gql.DeleteTeamInput{
		TeamId: teamID,
	})
	if err != nil {
		h.log.Error(err, "Failed to delete team", "hub", hubName, "team", teamName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to delete team",
		})
	}

	if len(resp.DeleteTeam.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.DeleteTeam.Errors))
	}

	return c.Status(fiber.StatusNoContent).SendString("")
}

// GetTeamStatus handles GET /hubs/:hub/teams/:team/status.
func (h *Handler) GetTeamStatus(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	fullTeamName := hubName + "--" + teamName

	ctx := h.contextWithIdentity(c)
	resp, err := gql.GetTeam(ctx, h.cpapi, &gql.TeamWhereInput{
		Name:         &fullTeamName,
		HasGroupWith: []gql.GroupWhereInput{{Name: &hubName}},
	})
	if err != nil {
		h.log.Error(err, "Failed to get team status", "hub", hubName, "team", teamName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to get team status",
		})
	}

	if len(resp.Teams.Edges) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Team not found: " + teamName,
		})
	}

	t := resp.Teams.Edges[0].Node
	status := mapper.TeamStatusToResourceStatus(t.StatusPhase, t.StatusMessage, t.CreatedAt, t.LastModifiedAt)
	return c.JSON(status)
}

// PatchTeamToken handles PATCH /hubs/:hub/teams/:team/teamToken.
func (h *Handler) PatchTeamToken(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")

	ctx := h.contextWithIdentity(c)

	teamID, err := h.resolveTeamID(ctx, hubName, teamName)
	if err != nil {
		return err
	}
	if teamID == "" {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Team not found: " + teamName,
		})
	}

	resp, err := gql.RotateTeamToken(ctx, h.cpapi, teamID)
	if err != nil {
		h.log.Error(err, "Failed to rotate team token", "hub", hubName, "team", teamName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to rotate team token",
		})
	}

	if len(resp.RotateTeamToken.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.RotateTeamToken.Errors))
	}

	t := resp.RotateTeamToken.Team
	result := fiber.Map{"teamToken": ""}
	if t.TeamToken != nil {
		result["teamToken"] = *t.TeamToken
	}
	return c.JSON(result)
}

// GetTeamResources handles GET /hubs/:hub/teams/:team/resources.
// Calls rover-server with a service token to retrieve resources for the team.
func (h *Handler) GetTeamResources(c *fiber.Ctx) error {
	hub := c.Params("hub")
	team := c.Params("team")
	id := mw.ConsumerIdentityFromContext(c)

	resources, err := h.rover.GetResources(c.UserContext(), id.Environment, hub, team)
	if err != nil {
		h.log.Error(err, "Failed to get team resources from rover-server")
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to retrieve resources from upstream",
		})
	}

	items := resources.Items
	p := mapper.ParsePagination(c)
	total := len(items)

	start := p.Offset
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	paged := items[start:end]

	c.Set("X-Total-Count", intToStr(total))
	c.Set("X-Result-Count", intToStr(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}
