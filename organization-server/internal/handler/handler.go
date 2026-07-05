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
func (h *Handler) RegisterRoutes(api fiber.Router) {
	// Hub (Group) CRUD
	api.Post("/hubs", h.CreateHub)
	api.Get("/hubs", h.ListHubs)
	api.Get("/hubs/:hub", h.GetHub)
	api.Put("/hubs/:hub", h.UpdateHub)
	api.Delete("/hubs/:hub", h.DeleteHub)
	api.Get("/hubs/:hub/status", h.GetHubStatus)

	// Team CRUD
	api.Post("/hubs/:hub/teams", h.CreateTeam)
	api.Get("/hubs/:hub/teams", h.ListTeams)
	api.Get("/hubs/:hub/teams/:team", h.GetTeam)
	api.Put("/hubs/:hub/teams/:team", h.UpdateTeam)
	api.Delete("/hubs/:hub/teams/:team", h.DeleteTeam)
	api.Get("/hubs/:hub/teams/:team/status", h.GetTeamStatus)

	// Team token
	api.Patch("/hubs/:hub/teams/:team/teamToken", h.PatchTeamToken)

	// Team resources (proxied to rover-server)
	api.Get("/hubs/:hub/teams/:team/resources", h.GetTeamResources)
}
