// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
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

// RegisterRoutes registers the GraphQL routes onto router, attaching guard
// (from the listener's security family) to /query. It fits the MultiServer
// RegisterFunc signature. The playground is registered separately and left
// unauthenticated (see RegisterPlayground).
func (c *Controller) RegisterRoutes(router fiber.Router, guard fiber.Handler) {
	c.RegisterPlayground(router, "/graphql")
	gqlHandler := httpHandlerWithUserContext(c.srv)
	group := router.Group("/graphql")
	group.Add(fiber.MethodPost, "/query", server.Guarded(guard, gqlHandler)...)
	group.Add(fiber.MethodGet, "/query", server.Guarded(guard, gqlHandler)...)
}

// RegisterPlayground registers the GraphQL playground on the given router
// without any security middleware, so it can be accessed from a browser.
func (c *Controller) RegisterPlayground(router fiber.Router, prefix string) {
	if c.playground {
		router.Get(prefix, adaptor.HTTPHandler(playground.Handler("ControlPlane API", prefix+"/query")))
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
		ctx := c.UserContext()

		// Propagate forwarded user identity headers into context for the Viewer middleware.
		// Values may be percent-encoded (encodeURIComponent) by the UI to support
		// non-ASCII characters in HTTP header values; decode them transparently.
		if name, email := decodeHeader(c.Get("X-Forwarded-User-Name")), decodeHeader(c.Get("X-Forwarded-User-Email")); name != "" || email != "" {
			fu := viewer.ForwardedUser{Name: name, Email: email}
			fu.IsAdmin = strings.EqualFold(c.Get("X-Forwarded-User-Is-Admin"), "true")
			if roles := decodeHeader(c.Get("X-Forwarded-User-Roles")); roles != "" {
				fu.Roles = strings.Split(roles, ",")
			}
			if groups := decodeHeader(c.Get("X-Forwarded-User-Groups")); groups != "" {
				fu.Groups = strings.Split(groups, ",")
			}
			ctx = viewer.NewForwardedUserContext(ctx, fu)
		}

		req = *req.WithContext(ctx)
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

// decodeHeader decodes a potentially percent-encoded header value.
// If decoding fails (e.g. the value was never encoded), it returns the original value.
func decodeHeader(value string) string {
	if decoded, err := url.QueryUnescape(value); err == nil {
		return decoded
	}
	return value
}
