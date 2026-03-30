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

func (s *Server) GetAllRoadmaps(c *fiber.Ctx) error {
	var params api.GetAllRoadmapsParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.Roadmaps.GetAll(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	if res.UnderscoreLinks.Self != "" {
		res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
		if res.UnderscoreLinks.Next != "" {
			res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
		}
	}

	return c.JSON(res)
}

func (s *Server) GetRoadmap(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.Roadmaps.Get(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateRoadmap(c *fiber.Ctx) error {
	var req api.RoadmapRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.Roadmaps.Create(c.UserContext(), req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusCreated).JSON(res)
}

func (s *Server) UpdateRoadmap(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.RoadmapRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.Roadmaps.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusOK).JSON(res)
}

func (s *Server) DeleteRoadmap(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.Roadmaps.Delete(c.UserContext(), resourceId); err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}
