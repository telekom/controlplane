// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
)

func (s *Server) CreateTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	var req api.TeamCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c)
	}

	ctx := c.UserContext()
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return forbidden(c)
	}

	result, mutErrs, err := s.ctrl.CreateTeam(ctx, bCtx.Environment, hubName, &req)
	if err != nil {
		return s.internalError(c, err, "Unable to create team", "hub", hubName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusAccepted).JSON(result)
}

func (s *Server) ListTeams(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	ctx := c.UserContext()

	teams, err := s.ctrl.ListTeams(ctx, hubName)
	if err != nil {
		return s.internalError(c, err, "Unable to list teams", "hub", hubName)
	}
	if bCtx, ok := security.FromContext(ctx); ok && bCtx.ClientType == security.ClientTypeTeam {
		teams = slices.DeleteFunc(teams, func(team api.TeamResponse) bool {
			return !strings.EqualFold(team.Name, bCtx.Team)
		})
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

	c.Set("X-Total-Count", strconv.Itoa(total))
	c.Set("X-Result-Count", strconv.Itoa(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}

func (s *Server) GetTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	ctx := c.UserContext()

	result, err := s.ctrl.GetTeam(ctx, hubName, teamName)
	if err != nil {
		return s.internalError(c, err, "Unable to get team", "hub", hubName, "team", teamName)
	}
	if result == nil {
		return notFound(c, "Team not found: "+teamName)
	}

	return c.JSON(result)
}

func (s *Server) UpdateTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")

	var req api.TeamUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c)
	}

	ctx := c.UserContext()

	result, mutErrs, err := s.ctrl.UpdateTeam(ctx, hubName, teamName, &req)
	if err != nil {
		return s.internalError(c, err, "Unable to update team", "hub", hubName, "team", teamName)
	}
	if result == nil && mutErrs == nil {
		return notFound(c, "Team not found: "+teamName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusAccepted).JSON(result)
}

func (s *Server) DeleteTeam(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	ctx := c.UserContext()

	mutErrs, err := s.ctrl.DeleteTeam(ctx, hubName, teamName)
	if err != nil {
		return s.internalError(c, err, "Unable to delete team", "hub", hubName, "team", teamName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusNoContent).SendString("")
}

func (s *Server) GetTeamStatus(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	ctx := c.UserContext()

	result, err := s.ctrl.GetTeamStatus(ctx, hubName, teamName)
	if err != nil {
		return s.internalError(c, err, "Unable to get team status", "hub", hubName, "team", teamName)
	}
	if result == nil {
		return notFound(c, "Team not found: "+teamName)
	}

	return c.JSON(result)
}

func (s *Server) PatchTeamToken(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	teamName := c.Params("team")
	ctx := c.UserContext()

	token, mutErrs, err := s.ctrl.RotateToken(ctx, hubName, teamName)
	if err != nil {
		return s.internalError(c, err, "Unable to rotate team token", "hub", hubName, "team", teamName)
	}
	if token == "" && mutErrs == nil {
		return notFound(c, "Team not found: "+teamName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.JSON(fiber.Map{"teamToken": token})
}

func (s *Server) GetTeamResources(c *fiber.Ctx) error {
	hub := c.Params("hub")
	team := c.Params("team")
	ctx := c.UserContext()
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return forbidden(c)
	}

	items, err := s.ctrl.GetResources(ctx, bCtx.Environment, hub, team)
	if err != nil {
		return s.internalError(c, err, "Unable to retrieve team resources", "hub", hub, "team", team)
	}

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

	c.Set("X-Total-Count", strconv.Itoa(total))
	c.Set("X-Result-Count", strconv.Itoa(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}
