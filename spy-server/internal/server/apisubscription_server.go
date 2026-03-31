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

// GetAllApiSubscriptions handles GET /applications/:applicationId/apisubscriptions
func (s *Server) GetAllApiSubscriptions(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")

	params := api.GetAllApiSubscriptionsParams{}
	if err := c.QueryParser(&params); err != nil {
		return cserver.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}

	resp, err := s.ApiSubscriptions.GetAll(c.UserContext(), applicationId, params)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}

	c.Set("X-Total-Count", strconv.Itoa(resp.Paging.Total))
	c.Set("X-Result-Count", strconv.Itoa(len(resp.Items)))
	return c.JSON(resp)
}

// GetApiSubscription handles GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName
func (s *Server) GetApiSubscription(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	apiSubscriptionName := c.Params("apiSubscriptionName")

	resp, err := s.ApiSubscriptions.Get(c.UserContext(), applicationId, apiSubscriptionName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}

// GetApiSubscriptionStatus handles GET /applications/:applicationId/apisubscriptions/:apiSubscriptionName/status
func (s *Server) GetApiSubscriptionStatus(c *fiber.Ctx) error {
	applicationId := c.Params("applicationId")
	apiSubscriptionName := c.Params("apiSubscriptionName")

	resp, err := s.ApiSubscriptions.GetStatus(c.UserContext(), applicationId, apiSubscriptionName)
	if err != nil {
		return cserver.ReturnWithProblem(c, nil, err)
	}
	return c.JSON(resp)
}
