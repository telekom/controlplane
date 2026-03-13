// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// Controller is a Fiber controller that wraps a gqlgen handler.
type Controller struct {
	srv        *handler.Server
	playground bool
}

// NewController creates a new GraphQL controller.
func NewController(srv *handler.Server, enablePlayground bool) *Controller {
	return &Controller{
		srv:        srv,
		playground: enablePlayground,
	}
}

// Register implements server.Controller.
func (c *Controller) Register(router fiber.Router, opts server.ControllerOpts) {
	checkAccess := security.ConfigureSecurity(router, opts.Security)
	router.Post("/query", checkAccess, adaptor.HTTPHandler(c.srv))
	router.Get("/query", checkAccess, adaptor.HTTPHandler(c.srv))
	if c.playground {
		router.Get("/", adaptor.HTTPHandler(playground.Handler("ControlPlane API", "/graphql/query")))
	}
}
