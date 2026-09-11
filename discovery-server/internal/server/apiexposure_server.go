// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/discovery-server/internal/api"
)

// GetAllApiExposures handles GET /applications/:applicationId/apiexposures
func (s *Server) GetAllApiExposures(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	params := api.GetAllApiExposuresParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.ApiExposures.GetAll(c.UserContext(), applicationId, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetApiExposure handles GET /applications/:applicationId/apiexposures/:apiExposureName
func (s *Server) GetApiExposure(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	apiExposureName := c.Params("apiExposureName")

	resp, err := s.ApiExposures.Get(c.UserContext(), applicationId, apiExposureName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetApiExposureStatus handles GET /applications/:applicationId/apiexposures/:apiExposureName/status
func (s *Server) GetApiExposureStatus(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	apiExposureName := c.Params("apiExposureName")

	resp, err := s.ApiExposures.GetStatus(c.UserContext(), applicationId, apiExposureName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetApiExposureSubscriptions handles GET /applications/:applicationId/apiexposures/:apiExposureName/apisubscriptions
func (s *Server) GetApiExposureSubscriptions(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	apiExposureName := c.Params("apiExposureName")

	params := api.GetAllExposureApiSubscriptionsParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.ApiExposures.GetSubscriptions(c.UserContext(), applicationId, apiExposureName, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}
