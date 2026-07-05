// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"github.com/gofiber/fiber/v2"
)

// CreateHub handles POST /hubs.
func (h *Handler) CreateHub(c *fiber.Ctx) error {
	// TODO: PR 5/6 — parse body, call CP API createGroup mutation, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub creation not yet implemented",
	})
}

// ListHubs handles GET /hubs.
func (h *Handler) ListHubs(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API listGroups, apply pagination, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub listing not yet implemented",
	})
}

// GetHub handles GET /hubs/:hub.
func (h *Handler) GetHub(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API getGroup, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub retrieval not yet implemented",
	})
}

// UpdateHub handles PUT /hubs/:hub.
func (h *Handler) UpdateHub(c *fiber.Ctx) error {
	// TODO: PR 5/6 — parse body, call CP API updateGroup, map response
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub update not yet implemented",
	})
}

// DeleteHub handles DELETE /hubs/:hub.
func (h *Handler) DeleteHub(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API deleteGroup
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub deletion not yet implemented",
	})
}

// GetHubStatus handles GET /hubs/:hub/status.
func (h *Handler) GetHubStatus(c *fiber.Ctx) error {
	// TODO: PR 5/6 — call CP API getGroupStatus, apply status mapping
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  "Not Implemented",
		"status": 501,
		"detail": "Hub status not yet implemented",
	})
}
