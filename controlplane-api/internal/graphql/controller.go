// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/valyala/fasthttp/fasthttpadaptor"
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
	gqlHandler := httpHandlerWithUserContext(c.srv)
	router.Post("/query", checkAccess, gqlHandler)
	router.Get("/query", checkAccess, gqlHandler)
	if c.playground {
		router.Get("/", adaptor.HTTPHandler(playground.Handler("ControlPlane API", "/graphql/query")))
	}
}

// httpHandlerWithUserContext wraps a net/http handler as a fiber handler,
// preserving the fiber UserContext (which carries the BusinessContext from security middleware).
// The default adaptor.HTTPHandler does not propagate UserContext.
func httpHandlerWithUserContext(h http.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req http.Request
		if err := fasthttpadaptor.ConvertRequest(c.Context(), &req, true); err != nil {
			return err
		}
		req = *req.WithContext(c.UserContext())
		rec := &responseRecorder{ctx: c}
		h.ServeHTTP(rec, &req)
		return nil
	}
}

// responseRecorder writes the http.Handler response back to the fiber context.
type responseRecorder struct {
	ctx *fiber.Ctx
}

func (r *responseRecorder) Header() http.Header {
	return http.Header{}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.ctx.Response().Header.Set("Content-Type", "application/json")
	return r.ctx.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.ctx.Status(statusCode)
}
