// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/organization-server/internal/api"
	"github.com/telekom/controlplane/organization-server/internal/mapper"
)

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
