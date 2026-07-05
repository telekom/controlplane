// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/organization-server/internal/api"
	gql "github.com/telekom/controlplane/organization-server/internal/graphql"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
)

// CreateHub handles POST /hubs.
func (h *Handler) CreateHub(c *fiber.Ctx) error {
	var req api.HubCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: float32(400),
			Detail: "Invalid request body",
		})
	}

	ctx := h.contextWithIdentity(c)
	desc := req.Description
	resp, err := gql.CreateGroup(ctx, h.cpapi, gql.CreateGroupInput{
		Environment: h.environment(c),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: &desc,
	})
	if err != nil {
		h.log.Error(err, "Failed to create group")
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to create hub",
		})
	}

	if len(resp.CreateGroup.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.CreateGroup.Errors))
	}

	g := resp.CreateGroup.Group
	return c.Status(fiber.StatusAccepted).JSON(api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	})
}

// ListHubs handles GET /hubs.
func (h *Handler) ListHubs(c *fiber.Ctx) error {
	ctx := h.contextWithIdentity(c)
	resp, err := gql.ListGroups(ctx, h.cpapi)
	if err != nil {
		h.log.Error(err, "Failed to list groups")
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to list hubs",
		})
	}

	hubs := make([]api.HubResponse, 0, len(resp.Groups))
	for i := range resp.Groups {
		hubs = append(hubs, mapper.GroupToHubResponse(&resp.Groups[i]))
	}

	p := mapper.ParsePagination(c)
	total := len(hubs)

	// Apply in-memory pagination.
	start := p.Offset
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	paged := hubs[start:end]

	c.Set("X-Total-Count", intToStr(total))
	c.Set("X-Result-Count", intToStr(len(paged)))
	return c.JSON(mapper.BuildPaginatedResponse(c, paged, total, p))
}

// GetHub handles GET /hubs/:hub.
func (h *Handler) GetHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	ctx := h.contextWithIdentity(c)
	resp, err := gql.GetGroup(ctx, h.cpapi, &gql.GroupWhereInput{
		Name: &hubName,
	})
	if err != nil {
		h.log.Error(err, "Failed to get group", "hub", hubName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to get hub",
		})
	}

	if len(resp.Groups) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Hub not found: " + hubName,
		})
	}

	g := &resp.Groups[0]
	return c.JSON(mapper.GroupDetailToHubResponse(g))
}

// UpdateHub handles PUT /hubs/:hub.
func (h *Handler) UpdateHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	var req api.HubUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: float32(400),
			Detail: "Invalid request body",
		})
	}

	ctx := h.contextWithIdentity(c)

	// Resolve group name to ID.
	groupID, err := h.resolveGroupID(ctx, hubName)
	if err != nil {
		return err
	}
	if groupID == "" {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Hub not found: " + hubName,
		})
	}

	resp, err := gql.UpdateGroup(ctx, h.cpapi, gql.UpdateGroupInput{
		GroupId:     groupID,
		DisplayName: &req.DisplayName,
		Description: &req.Description,
	})
	if err != nil {
		h.log.Error(err, "Failed to update group", "hub", hubName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to update hub",
		})
	}

	if len(resp.UpdateGroup.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.UpdateGroup.Errors))
	}

	g := resp.UpdateGroup.Group
	return c.Status(fiber.StatusAccepted).JSON(api.HubResponse{
		Name:        g.Name,
		DisplayName: g.DisplayName,
		Description: g.Description,
		Status: api.Status{
			ProcessingState: "pending",
			State:           "none",
		},
	})
}

// DeleteHub handles DELETE /hubs/:hub.
func (h *Handler) DeleteHub(c *fiber.Ctx) error {
	hubName := c.Params("hub")

	ctx := h.contextWithIdentity(c)

	groupID, err := h.resolveGroupID(ctx, hubName)
	if err != nil {
		return err
	}
	if groupID == "" {
		return c.Status(fiber.StatusNotFound).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Not Found",
			Status: float32(404),
			Detail: "Hub not found: " + hubName,
		})
	}

	resp, err := gql.DeleteGroup(ctx, h.cpapi, gql.DeleteGroupInput{
		GroupId: groupID,
	})
	if err != nil {
		h.log.Error(err, "Failed to delete group", "hub", hubName)
		return c.Status(fiber.StatusBadGateway).JSON(api.Error{
			Type:   "about:blank",
			Title:  "Bad Gateway",
			Status: float32(502),
			Detail: "Unable to delete hub",
		})
	}

	if len(resp.DeleteGroup.Errors) > 0 {
		return h.mapMutationErrors(c, toMutationErrors(resp.DeleteGroup.Errors))
	}

	return c.Status(fiber.StatusNoContent).SendString("")
}

// GetHubStatus handles GET /hubs/:hub/status.
func (h *Handler) GetHubStatus(c *fiber.Ctx) error {
	// Groups don't have a status phase in the current model.
	// Return a static "done/complete" since groups are always immediately ready.
	return c.JSON(api.ResourceStatusResponse{
		OverallStatus:   "done",
		ProcessingState: api.ProcessingStateDone,
		State:           api.StateComplete,
	})
}
