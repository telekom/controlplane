// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"strconv"

	"github.com/Khan/genqlient/graphql"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/client"
	"github.com/telekom/controlplane/organization-server/internal/controller"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
	mw "github.com/telekom/controlplane/organization-server/internal/middleware"
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
func (s *Server) RegisterRoutes(router fiber.Router, teamAuth fiber.Handler) {
	router.Post("/hubs", s.CreateHub)
	router.Get("/hubs", s.ListHubs)
	router.Get("/hubs/:hub", teamAuth, s.GetHub)
	router.Put("/hubs/:hub", teamAuth, s.UpdateHub)
	router.Delete("/hubs/:hub", teamAuth, s.DeleteHub)
	router.Get("/hubs/:hub/status", teamAuth, s.GetHubStatus)

	router.Post("/hubs/:hub/teams", teamAuth, s.CreateTeam)
	router.Get("/hubs/:hub/teams", teamAuth, s.ListTeams)
	router.Get("/hubs/:hub/teams/:team", teamAuth, s.GetTeam)
	router.Put("/hubs/:hub/teams/:team", teamAuth, s.UpdateTeam)
	router.Delete("/hubs/:hub/teams/:team", teamAuth, s.DeleteTeam)
	router.Get("/hubs/:hub/teams/:team/status", teamAuth, s.GetTeamStatus)

	router.Patch("/hubs/:hub/teams/:team/teamToken", teamAuth, s.PatchTeamToken)
	router.Get("/hubs/:hub/teams/:team/resources", teamAuth, s.GetTeamResources)
}

// --- Hub handlers ---

func (s *Server) CreateHub(c *fiber.Ctx) error {
	var req api.HubCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c)
	}

	ctx := s.contextWithIdentity(c)
	id := mw.ConsumerIdentityFromContext(c)

	result, mutErrs, err := s.ctrl.Create(ctx, id.Environment, &req)
	if err != nil {
		return s.internalError(c, err, "Unable to create hub", "hub", req.Name)
	}
	if mutErrs != nil {
		return mapMutationErrors(c, mutErrs)
	}

	return c.Status(fiber.StatusAccepted).JSON(result)
}

func (s *Server) ListHubs(c *fiber.Ctx) error {
	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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

	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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

	ctx := s.contextWithIdentity(c)
	id := mw.ConsumerIdentityFromContext(c)

	result, mutErrs, err := s.ctrl.CreateTeam(ctx, id.Environment, hubName, &req)
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
	ctx := s.contextWithIdentity(c)

	teams, err := s.ctrl.ListTeams(ctx, hubName)
	if err != nil {
		return s.internalError(c, err, "Unable to list teams", "hub", hubName)
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
	ctx := s.contextWithIdentity(c)

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

	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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
	ctx := s.contextWithIdentity(c)

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
	id := mw.ConsumerIdentityFromContext(c)
	ctx := c.UserContext()

	items, err := s.ctrl.GetResources(ctx, id.Environment, hub, team)
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

func (s *Server) contextWithIdentity(c *fiber.Ctx) context.Context {
	id := mw.ConsumerIdentityFromContext(c)
	ctx := c.UserContext()
	if id != nil {
		ctx = client.WithIdentity(ctx, &client.ConsumerIdentity{
			Environment: id.Environment,
			Group:       id.Group,
			Team:        id.Team,
		})
	}
	return ctx
}

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
