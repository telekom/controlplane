// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"

	"github.com/telekom/controlplane/spy-server/internal/api"
)

// GetAllApplications handles GET /applications
func (s *Server) GetAllApplications(c *fiber.Ctx) error {
	params := api.GetAllApplicationsParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.Applications.GetAll(c.UserContext(), params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetApplication handles GET /applications/:applicationId
func (s *Server) GetApplication(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	resp, err := s.Applications.Get(c.UserContext(), applicationId)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetApplicationStatus handles GET /applications/:applicationId/status
func (s *Server) GetApplicationStatus(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	resp, err := s.Applications.GetStatus(c.UserContext(), applicationId)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}
