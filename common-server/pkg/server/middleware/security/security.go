package security

import (
	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

type contextKey string

const (
	businessContextKey contextKey = "businessContext"
	prefixKey          contextKey = "prefix"
)

type Option[T any] func(T)

type SecurityOpts struct {
	Enabled bool
	Log     logr.Logger

	JWTOpts             []Option[*JWTOpts]
	BusinessContextOpts []Option[*BusinessContextOpts]
	CheckAccessOpts     []Option[*CheckAccessOpts]
}

// ConfigureSecurity configures the security middlewares
// This is a convenience function that configures the JWT middleware
// and the business context middleware
// It also returns the check access middleware that should be configured on the route level
func ConfigureSecurity(router fiber.Router, opts SecurityOpts) fiber.Handler {
	if !opts.Enabled {
		opts.Log.Info("⚠️ Security middleware disabled")
		return func(c *fiber.Ctx) error {
			ctxLog := opts.Log.WithName("mock")
			ctx := logr.NewContext(c.UserContext(), ctxLog)
			c.SetUserContext(ctx)
			return c.Next()
		}
	}
	opts.Log.Info("🔑 Security middleware enabled")

	busCtx := NewBusinessCtxMiddlewareWithOpts(opts.BusinessContextOpts...)
	checkAccess := NewCheckAccessMiddlewareWithOpts(opts.CheckAccessOpts...)

	if IsMock(opts.JWTOpts) {
		opts.Log.Info("⚠️ Security middleware mocked")
		router.Use(mock.NewJWTMock())
	} else {
		router.Use(NewJWTWithOpts(opts.JWTOpts...))
	}

	router.Use(busCtx)
	return checkAccess
}

// IsMock checks if the security middleware is mocked
// If the trusted issuers are not set, the middleware is considered mocked
func IsMock(opts []Option[*JWTOpts]) bool {
	jwtOpts := JWTOpts{}
	for _, f := range opts {
		f(&jwtOpts)
	}
	return len(jwtOpts.TrustedIssuers) == 0
}
