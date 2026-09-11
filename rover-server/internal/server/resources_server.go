// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"net/url"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/rover-server/internal/api"
)

func (s *Server) GetAllResources(c *fiber.Ctx) error {
	var params api.GetAllResourcesParams
	if err := c.QueryParser(&params); err != nil {
		return server.ReturnWithProblem(c, problems.BadRequest("invalid query parameters"), err)
	}
	if params.Limit == 0 {
		params.Limit = 20
	}
	res, err := s.Resources.GetAll(c.UserContext(), params)
	if err != nil {
		var problem problems.Problem
		if errors.As(err, &problem) {
			return server.ReturnWithProblem(c, problem, nil)
		}
		logr.FromContextOrDiscard(c.UserContext()).Error(err, "Failed to list team resources")
		return server.ReturnWithProblem(c, problems.InternalServerError("Unable to list team resources", "The resource inventory could not be loaded"), nil)
	}

	group, team := params.Group, params.Team
	if group == "" {
		businessContext, _ := security.FromContext(c.UserContext())
		group, team = businessContext.Group, businessContext.Team
	}
	res.UnderscoreLinks.Self = resourceURL(c, group, team, params.Limit, res.UnderscoreLinks.Self)
	if res.UnderscoreLinks.Next != "" {
		res.UnderscoreLinks.Next = resourceURL(c, group, team, params.Limit, res.UnderscoreLinks.Next)
	}

	return c.JSON(res)
}

func resourceURL(c *fiber.Ctx, group, team string, limit int32, cursor string) string {
	u := &url.URL{Path: c.Path()}
	query := u.Query()
	query.Set("group", group)
	query.Set("team", team)
	query.Set("limit", strconv.FormatInt(int64(limit), 10))
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	u.RawQuery = query.Encode()
	return c.BaseURL() + u.String()
}
