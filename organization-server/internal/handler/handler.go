// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"github.com/Khan/genqlient/graphql"
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/client"
)

// Handler groups all endpoint handlers for the organization facade.
type Handler struct {
	cpapi graphql.Client
	rover *client.RoverClient
	log   logr.Logger
}

// New creates a new Handler with the given upstream clients.
func New(cpapi graphql.Client, rover *client.RoverClient, log logr.Logger) *Handler {
	return &Handler{
		cpapi: cpapi,
		rover: rover,
		log:   log,
	}
}

// RegisterRoutes registers all REST endpoints on the given Fiber router group.
// The router group is expected to be mounted at /organization/v1.
// teamAuth is applied to routes that access hub/team-scoped resources.
func (h *Handler) RegisterRoutes(api fiber.Router, teamAuth fiber.Handler) {
	// Hub (Group) CRUD — hub-level auth
	api.Post("/hubs", h.CreateHub)
	api.Get("/hubs", h.ListHubs)
	api.Get("/hubs/:hub", teamAuth, h.GetHub)
	api.Put("/hubs/:hub", teamAuth, h.UpdateHub)
	api.Delete("/hubs/:hub", teamAuth, h.DeleteHub)
	api.Get("/hubs/:hub/status", teamAuth, h.GetHubStatus)

	// Team CRUD — hub+team level auth
	api.Post("/hubs/:hub/teams", teamAuth, h.CreateTeam)
	api.Get("/hubs/:hub/teams", teamAuth, h.ListTeams)
	api.Get("/hubs/:hub/teams/:team", teamAuth, h.GetTeam)
	api.Put("/hubs/:hub/teams/:team", teamAuth, h.UpdateTeam)
	api.Delete("/hubs/:hub/teams/:team", teamAuth, h.DeleteTeam)
	api.Get("/hubs/:hub/teams/:team/status", teamAuth, h.GetTeamStatus)

	// Team token
	api.Patch("/hubs/:hub/teams/:team/teamToken", teamAuth, h.PatchTeamToken)

	// Team resources (proxied to rover-server)
	api.Get("/hubs/:hub/teams/:team/resources", teamAuth, h.GetTeamResources)
}
