// Copyright 2026 Deutsche Telekom IT GmbH
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

func (s *Server) GetAllFileSpecifications(c *fiber.Ctx) error {
	var params api.GetAllFileSpecificationsParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.FileSpecifications.GetAll(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func (s *Server) GetFileSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.FileSpecifications.Get(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateFileSpecification(c *fiber.Ctx) error {
	var req api.FileSpecificationCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.FileSpecifications.Create(c.UserContext(), req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateFileSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.FileSpecificationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.FileSpecifications.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteFileSpecification(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.FileSpecifications.Delete(c.UserContext(), resourceId); err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetFileSpecificationStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.FileSpecifications.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}
