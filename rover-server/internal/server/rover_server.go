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

func (s *Server) GetAllRovers(c *fiber.Ctx) error {
	var params api.GetAllRoversParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.Rovers.GetAll(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	res.UnderscoreLinks.Self = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = buildCursorUrl(c.BaseURL(), c.Path(), res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func (s *Server) GetRover(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.Rovers.Get(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) CreateRover(c *fiber.Ctx) error {
	var req api.RoverCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.Rovers.Create(c.UserContext(), req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) UpdateRover(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var req api.RoverUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid request body"), err)
	}

	res, err := s.Rovers.Update(c.UserContext(), resourceId, req)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}

func (s *Server) DeleteRover(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	if err := s.Rovers.Delete(c.UserContext(), resourceId); err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.SendStatus(http.StatusNoContent)
}

func (s *Server) GetRoverStatus(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.Rovers.GetStatus(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) GetApplicationInfo(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	var params api.GetApplicationInfoParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.Rovers.GetApplicationInfo(c.UserContext(), resourceId, params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) GetManyApplicationInfo(c *fiber.Ctx) error {
	var params api.GetApplicationsInfoParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	res, err := s.Rovers.GetApplicationsInfo(c.UserContext(), params)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.JSON(res)
}

func (s *Server) ResetRoverSecret(c *fiber.Ctx) error {
	resourceId := c.Params("resourceId")
	res, err := s.Rovers.ResetRoverSecret(c.UserContext(), resourceId)
	if err != nil {
		return server.ReturnWithProblem(c, nil, err)
	}

	return c.Status(http.StatusAccepted).JSON(res)
}
