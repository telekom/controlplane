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

func (s *Server) GetAllAgentSpecifications(c *fiber.Ctx) error {
	var params api.GetAllAgentSpecificationsParams
	if err := c.QueryParser(&params); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.AgentSpecifications.GetAll(c.UserContext(), params)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func (s *Server) GetAgentSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.AgentSpecifications.Get(c.UserContext(), resourceId)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateAgentSpecification(c *fiber.Ctx) error {
	var req api.AgentSpecificationCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.AgentSpecifications.Create(c.UserContext(), req)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateAgentSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.AgentSpecificationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return commonserver.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.AgentSpecifications.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteAgentSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.AgentSpecifications.Delete(c.UserContext(), resourceId); err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetAgentSpecificationStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.AgentSpecifications.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return commonserver.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}
