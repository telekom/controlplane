// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/Khan/genqlient/graphql"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/controller"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
)

// Server is the thin HTTP layer that parses requests, delegates to the controller,
// and formats responses.
type Server struct {
	ctrl *controller.Controller
	log  logr.Logger
}

// New creates a new Server with the given controller and logger.
func New(ctrl *controller.Controller, log logr.Logger) *Server {
	return &Server{
		ctrl: ctrl,
		log:  log,
	}
}

// RegisterRoutes registers all REST endpoints on the given Fiber router group.
func (s *Server) RegisterRoutes(router fiber.Router, guard fiber.Handler) {
	router.Add(fiber.MethodPost, "/hubs", append(cserver.Guarded(guard, adminCreateOnly), s.CreateHub)...)
	router.Add(fiber.MethodGet, "/hubs", cserver.Guarded(guard, s.ListHubs)...)
	router.Add(fiber.MethodGet, "/hubs/:hub", cserver.Guarded(guard, s.GetHub)...)
	router.Add(fiber.MethodPut, "/hubs/:hub", cserver.Guarded(guard, s.UpdateHub)...)
	router.Add(fiber.MethodDelete, "/hubs/:hub", cserver.Guarded(guard, s.DeleteHub)...)
	router.Add(fiber.MethodGet, "/hubs/:hub/status", cserver.Guarded(guard, s.GetHubStatus)...)

	router.Add(fiber.MethodPost, "/hubs/:hub/teams", append(cserver.Guarded(guard, adminCreateOnly), s.CreateTeam)...)
	router.Add(fiber.MethodGet, "/hubs/:hub/teams", cserver.Guarded(guard, s.ListTeams)...)
	router.Add(fiber.MethodGet, "/hubs/:hub/teams/:team", cserver.Guarded(guard, s.GetTeam)...)
	router.Add(fiber.MethodPut, "/hubs/:hub/teams/:team", cserver.Guarded(guard, s.UpdateTeam)...)
	router.Add(fiber.MethodDelete, "/hubs/:hub/teams/:team", cserver.Guarded(guard, s.DeleteTeam)...)
	router.Add(fiber.MethodGet, "/hubs/:hub/teams/:team/status", cserver.Guarded(guard, s.GetTeamStatus)...)

	router.Add(fiber.MethodPatch, "/hubs/:hub/teams/:team/teamToken", cserver.Guarded(guard, s.PatchTeamToken)...)
	router.Add(fiber.MethodGet, "/hubs/:hub/teams/:team/resources", cserver.Guarded(guard, s.GetTeamResources)...)
}

func adminCreateOnly(c *fiber.Ctx) error {
	bCtx, ok := security.FromContext(c.UserContext())
	if !ok || bCtx.ClientType != security.ClientTypeAdmin || !bCtx.AccessType.IsWrite() {
		p := problems.Forbidden("Forbidden", "Admin write access required")
		return c.Status(p.Code()).JSON(p, "application/problem+json")
	}
	return c.Next()
}

// --- Hub handlers ---

func (s *Server) CreateHub(c *fiber.Ctx) error {
	var req api.HubCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c)
	}

	ctx := c.UserContext()
	bCtx, ok := security.FromContext(ctx)
	if !ok {
		return forbidden(c)
	}

	result, mutErrs, err := s.ctrl.Create(ctx, bCtx.Environment, &req)
	if err != nil {
		return s.internalError(c, err, "Unable to create hub", "hub", req.Name)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusAccepted).JSON(result)
}

func (s *Server) ListHubs(c *fiber.Ctx) error {
	ctx := c.UserContext()

	hubs, err := s.ctrl.List(ctx)
	if err != nil {
		return s.internalError(c, err, "Unable to list hubs")
	}

	p := mapper.ParsePagination(c)
	total := len(hubs)

	start := p.Offset
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	paged := hubs[start:end]

	c.Set("X-Total-Count", strconv.Itoa(total))
	c.Set("X-Result-Count", strconv.Itoa(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}

func (s *Server) GetHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	ctx := c.UserContext()

	result, err := s.ctrl.Get(ctx, hubName)
	if err != nil {
		return s.internalError(c, err, "Unable to get hub", "hub", hubName)
	}
	if result == nil {
		return notFound(c, "Hub not found: "+hubName)
	}

	return c.JSON(result)
}

func (s *Server) UpdateHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	var req api.HubUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c)
	}

	ctx := c.UserContext()

	result, mutErrs, err := s.ctrl.Update(ctx, hubName, &req)
	if err != nil {
		return s.internalError(c, err, "Unable to update hub", "hub", hubName)
	}
	if result == nil && mutErrs == nil {
		return notFound(c, "Hub not found: "+hubName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusAccepted).JSON(result)
}

func (s *Server) DeleteHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	ctx := c.UserContext()

	mutErrs, err := s.ctrl.Delete(ctx, hubName)
	if err != nil {
		return s.internalError(c, err, "Unable to delete hub", "hub", hubName)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusNoContent).SendString("")
}

func (s *Server) GetHubStatus(c *fiber.Ctx) error {
	hubName := c.Params("hub")
	ctx := c.UserContext()

	result, err := s.ctrl.GetStatus(ctx, hubName)
	if err != nil {
		return s.internalError(c, err, "Unable to get hub status", "hub", hubName)
	}

	return c.JSON(result)
}

// --- Team handlers ---

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

// --- helpers ---

func (s *Server) internalError(c *fiber.Ctx, err error, msg string, keysAndValues ...any) error {
	var httpErr *graphql.HTTPError
	if errors.As(err, &httpErr) {
		keysAndValues = append(keysAndValues, "upstreamStatus", httpErr.StatusCode)
	}
	s.log.Error(err, msg, keysAndValues...)

	return c.Status(fiber.StatusInternalServerError).JSON(api.Error{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Status: float32(500),
		Detail: msg,
	})
}

func badRequest(c *fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(api.Error{
		Type:   "about:blank",
		Title:  "Bad Request",
		Status: float32(400),
		Detail: "Invalid request body",
	})
}

func forbidden(c *fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).JSON(api.Error{
		Type:   "about:blank",
		Title:  "Forbidden",
		Status: float32(fiber.StatusForbidden),
		Detail: "No business context",
	})
}

func notFound(c *fiber.Ctx, detail string) error {
	return c.Status(fiber.StatusNotFound).JSON(api.Error{
		Type:   "about:blank",
		Title:  "Not Found",
		Status: float32(404),
		Detail: detail,
	})
}

func mapMutationErrors(c *fiber.Ctx, errs []controller.MutationError) error {
	if len(errs) == 0 {
		return nil
	}

	e := errs[0]
	status := fiber.StatusInternalServerError
	switch e.Code {
	case "FORBIDDEN":
		status = fiber.StatusForbidden
	case "NOT_FOUND":
		status = fiber.StatusNotFound
	case "CONFLICT", "ALREADY_EXISTS":
		status = fiber.StatusConflict
	case "PRECONDITION_FAILED":
		status = fiber.StatusPreconditionFailed
	case "BAD_REQUEST", "VALIDATION_FAILED":
		status = fiber.StatusBadRequest
	}

	return c.Status(status).JSON(api.Error{
		Type:   "about:blank",
		Title:  e.Code,
		Status: float32(status),
		Detail: e.Message,
	})
}
