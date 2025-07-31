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

func (s *Server) GetAllApiSpecifications(c *fiber.Ctx) error {
	var params api.GetAllApiSpecificationsParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.ApiSpecifications.GetAll(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func (s *Server) GetApiSpecifications(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.ApiSpecifications.Get(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateApiSpecification(c *fiber.Ctx) error {
	var req api.ApiSpecificationCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.ApiSpecifications.Create(c.UserContext(), req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateApiSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.ApiSpecificationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.ApiSpecifications.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteApiSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.ApiSpecifications.Delete(c.UserContext(), resourceId); err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetApiSpecificationStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.ApiSpecifications.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}
