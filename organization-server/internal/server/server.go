// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"

	"github.com/Khan/genqlient/graphql"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/organization-server/internal/controller"
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

// --- helpers ---

func (s *Server) internalError(c *fiber.Ctx, err error, msg string, keysAndValues ...any) error {
	var httpErr *graphql.HTTPError
	if errors.As(err, &httpErr) {
		keysAndValues = append(keysAndValues, "upstreamStatus", httpErr.StatusCode)
	}
	s.log.Error(err, msg, keysAndValues...)

	return cserver.ReturnWithProblem(c, problems.InternalServerError("Internal Server Error", msg), nil)
}

func badRequest(c *fiber.Ctx) error {
	return cserver.ReturnWithProblem(c, problems.BadRequest("Invalid request body"), nil)
}

func forbidden(c *fiber.Ctx) error {
	return cserver.ReturnWithProblem(c, problems.Forbidden("Forbidden", "No business context"), nil)
}

func notFound(c *fiber.Ctx, detail string) error {
	problem := problems.Builder().
		Status(fiber.StatusNotFound).
		Type("about:blank").
		Title("Not Found").
		Detail(detail).
		Build()
	return cserver.ReturnWithProblem(c, problem, nil)
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

	problem := problems.Builder().
		Status(status).
		Type("about:blank").
		Title(e.Code).
		Detail(e.Message).
		Build()
	return cserver.ReturnWithProblem(c, problem, nil)
}
