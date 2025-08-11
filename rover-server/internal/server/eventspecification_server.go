// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func (s *Server) GetAllEventSpecifications(c *fiber.Ctx) error {
	var params api.GetAllEventSpecificationsParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.EventSpecifications.GetAll(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) GetEventSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.EventSpecifications.Get(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateEventSpecification(c *fiber.Ctx) error {
	var req api.EventSpecificationCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.EventSpecifications.Create(c.UserContext(), req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateEventSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.EventSpecificationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.EventSpecifications.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteEventSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.EventSpecifications.Delete(c.UserContext(), resourceId); err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetEventSpecificationStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.EventSpecifications.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}
