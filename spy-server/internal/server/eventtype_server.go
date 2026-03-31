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

// GetAllEventTypes handles GET /eventtypes
func (s *Server) GetAllEventTypes(c *fiber.Ctx) error {
	params := api.GetAllEventTypesParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.EventTypes.GetAll(c.UserContext(), params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetEventType handles GET /eventtypes/:eventTypeId
func (s *Server) GetEventType(c *fiber.Ctx) error {
	eventTypeId := c.Params("eventTypeId")

	resp, err := s.EventTypes.Get(c.UserContext(), eventTypeId)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetEventTypeStatus handles GET /eventtypes/:eventTypeId/status
func (s *Server) GetEventTypeStatus(c *fiber.Ctx) error {
	eventTypeId := c.Params("eventTypeId")

	resp, err := s.EventTypes.GetStatus(c.UserContext(), eventTypeId)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetActiveEventType handles GET /eventtypes/:eventTypeName/active
// We can use eventTypeName instead of eventTypeId here,
// because there can only be one active event type with a given name.
func (s *Server) GetActiveEventType(c *fiber.Ctx) error {
	eventTypeName := c.Params("eventTypeName")

	resp, err := s.EventTypes.GetActive(c.UserContext(), eventTypeName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}
