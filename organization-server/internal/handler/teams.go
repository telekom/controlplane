// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"github.com/gofiber/fiber/v2"

	mw "github.com/telekom/controlplane/organization-server/internal/middleware"
)

// CreateTeam handles POST /hubs/:hub/teams.
func (h *Handler) CreateTeam(c *fiber.Ctx) error {
	// TODO: PR 5/6 — parse body, call CP API createTeam mutation, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team creation not yet implemented",
	})
}

// ListTeams handles GET /hubs/:hub/teams.
func (h *Handler) ListTeams(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API listTeams(group), apply pagination, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team listing not yet implemented",
	})
}

// GetTeam handles GET /hubs/:hub/teams/:team.
func (h *Handler) GetTeam(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API getTeam, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team retrieval not yet implemented",
	})
}

// UpdateTeam handles PUT /hubs/:hub/teams/:team.
func (h *Handler) UpdateTeam(c *fiber.Ctx) error {
	// TODO: PR 5/6 — parse body, call CP API updateTeam, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team update not yet implemented",
	})
}

// DeleteTeam handles DELETE /hubs/:hub/teams/:team.
func (h *Handler) DeleteTeam(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API deleteTeam
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team deletion not yet implemented",
	})
}

// GetTeamStatus handles GET /hubs/:hub/teams/:team/status.
func (h *Handler) GetTeamStatus(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API getTeamStatus, apply status mapping
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team status not yet implemented",
	})
}

// PatchTeamToken handles PATCH /hubs/:hub/teams/:team/teamToken.
func (h *Handler) PatchTeamToken(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API rotateTeamToken, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Team token rotation not yet implemented",
	})
}

// GetTeamResources handles GET /hubs/:hub/teams/:team/resources.
// This proxies directly to rover-server using the consumer's token.
func (h *Handler) GetTeamResources(c *fiber.Ctx) error {
	token := mw.RawTokenFromContext(c)

	resources, err := h.rover.GetResources(c.UserContext(), token, "")
	if err != nil {
		h.log.Error(err, "Failed to get team resources from rover-server")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"type":   "about:blank",
			"title":  "Bad Gateway",
			"status": 502,
			"detail": "Unable to retrieve resources from upstream",
		})
	}

	// Apply pagination (in-memory for small datasets).
	items := resources.Items
	offset := c.QueryInt("offset", 0)
	limit := c.QueryInt("limit", 20)
	total := len(items)

	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	paged := items[offset:end]

	page := (offset / limit) + 1
	lastPage := (total + limit - 1) / limit
	if lastPage < 1 {
		lastPage = 1
	}

	return c.JSON(fiber.Map{
		"_links": fiber.Map{
			"first": buildPaginationLink(c, 0, limit),
			"last":  buildPaginationLink(c, (lastPage-1)*limit, limit),
			"self":  buildPaginationLink(c, offset, limit),
		},
		"items": paged,
		"paging": fiber.Map{
			"last_page": lastPage,
			"page":      page,
			"total":     total,
		},
	})
}

func buildPaginationLink(c *fiber.Ctx, offset, limit int) string {
	return c.Path() + "?offset=" + intToStr(offset) + "&limit=" + intToStr(limit)
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
