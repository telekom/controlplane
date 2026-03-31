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

// GetAllEventSubscriptions handles GET /applications/:applicationId/eventsubscriptions
func (s *Server) GetAllEventSubscriptions(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	params := api.GetAllEventSubscriptionsParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.EventSubscriptions.GetAll(c.UserContext(), applicationId, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetEventSubscription handles GET /applications/:applicationId/eventsubscriptions/:eventSubscriptionName
func (s *Server) GetEventSubscription(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	eventSubscriptionName := c.Params("eventSubscriptionName")

	resp, err := s.EventSubscriptions.Get(c.UserContext(), applicationId, eventSubscriptionName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetEventSubscriptionStatus handles GET /applications/:applicationId/eventsubscriptions/:eventSubscriptionName/status
func (s *Server) GetEventSubscriptionStatus(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	eventSubscriptionName := c.Params("eventSubscriptionName")

	resp, err := s.EventSubscriptions.GetStatus(c.UserContext(), applicationId, eventSubscriptionName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}
