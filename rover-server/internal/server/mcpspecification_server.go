// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	commonserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/rover-server/internal/api"
)

func (s *Server) GetAllMcpSpecifications(c *fiber.Ctx) error {
	var params api.GetAllMcpSpecificationsParams
	if err := c.QueryParser(&params); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.McpSpecifications.GetAll(c.UserContext(), params)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func (s *Server) GetMcpSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.McpSpecifications.Get(c.UserContext(), resourceId)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateMcpSpecification(c *fiber.Ctx) error {
	var req api.McpSpecificationCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.McpSpecifications.Create(c.UserContext(), req)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateMcpSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.McpSpecificationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.McpSpecifications.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteMcpSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.McpSpecifications.Delete(c.UserContext(), resourceId); err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetMcpSpecificationStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.McpSpecifications.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}
