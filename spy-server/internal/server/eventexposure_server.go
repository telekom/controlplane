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

// GetAllEventExposures handles GET /applications/:applicationId/eventexposures
func (s *Server) GetAllEventExposures(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	params := api.GetAllEventExposuresParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.EventExposures.GetAll(c.UserContext(), applicationId, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetEventExposure handles GET /applications/:applicationId/eventexposures/:eventExposureName
func (s *Server) GetEventExposure(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	eventExposureName := c.Params("eventExposureName")

	resp, err := s.EventExposures.Get(c.UserContext(), applicationId, eventExposureName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetEventExposureStatus handles GET /applications/:applicationId/eventexposures/:eventExposureName/status
func (s *Server) GetEventExposureStatus(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	eventExposureName := c.Params("eventExposureName")

	resp, err := s.EventExposures.GetStatus(c.UserContext(), applicationId, eventExposureName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetEventExposureSubscriptions handles GET /applications/:applicationId/eventexposures/:eventExposureName/eventsubscriptions
func (s *Server) GetEventExposureSubscriptions(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	eventExposureName := c.Params("eventExposureName")

	params := api.GetAllExposureEventSubscriptionsParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.EventExposures.GetSubscriptions(c.UserContext(), applicationId, eventExposureName, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}
