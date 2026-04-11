// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

func (s *Server) GetAllApiRoadmaps(c *fiber.Ctx) error {
	var params api.GetAllApiRoadmapsParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	return s.Roadmaps.GetAllApiRoadmaps(c, params)
}

func (s *Server) GetApiRoadmap(c *fiber.Ctx) error {
	apiRoadmapId := c.Params("resourceId")
	return s.Roadmaps.GetApiRoadmap(c, apiRoadmapId)
}

func (s *Server) CreateApiRoadmap(c *fiber.Ctx) error {
	return s.Roadmaps.CreateApiRoadmap(c)
}

func (s *Server) UpdateApiRoadmap(c *fiber.Ctx) error {
	apiRoadmapId := c.Params("resourceId")
	return s.Roadmaps.UpdateApiRoadmap(c, apiRoadmapId)
}

func (s *Server) DeleteApiRoadmap(c *fiber.Ctx) error {
	apiRoadmapId := c.Params("resourceId")
	return s.Roadmaps.DeleteApiRoadmap(c, apiRoadmapId)
}
