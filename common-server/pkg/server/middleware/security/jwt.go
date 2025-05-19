package security

import (
	"strings"

	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"
)

var UserContextKey = "user"

func mapIssuerUrls(issuers []string) []string {
	jwkUrls := make([]string, 0, len(issuers))
	for _, issuer := range issuers {
		jwkUrls = append(jwkUrls, issuer+"/protocol/openid-connect/certs")
	}
	return jwkUrls
}

type JWTOpts struct {
	TrustedIssuers  []string
	PerformLMSCheck bool
	LmsBasePath     string
}

func WithTrustedIssuers(issuers []string) Option[*JWTOpts] {
	return func(o *JWTOpts) {
		o.TrustedIssuers = issuers
	}
}

func WithLmsCheck(basePath string) Option[*JWTOpts] {
	return func(o *JWTOpts) {
		o.PerformLMSCheck = basePath != ""
		o.LmsBasePath = basePath
	}
}

func NewJWTWithOpts(opts ...Option[*JWTOpts]) fiber.Handler {
	jwtOpts := JWTOpts{}

	for _, f := range opts {
		f(&jwtOpts)
	}

	return NewJWT(jwtOpts)
}

// NewJWT creates a new JWT middleware
// This middleware will validate the JWT token and
// perform the Last-Mile-Security(LMS)-Check
// The LMS-Check is performed by checking the operation and requestPath claims
func NewJWT(opts JWTOpts) fiber.Handler {
	lmsChecks := func(claims jwt.MapClaims, method, path string) error {
		// check if operation claim and requestPath Claim are available
		operationClaim, ok := util.NotNilOfType[string](claims["operation"])
		if !ok {
			return problems.Forbidden("Access denied", "operation claim is invalid")
		}
		requestPathClaim, ok := util.NotNilOfType[string](claims["requestPath"])
		if !ok {
			return problems.Forbidden("Access denied", "requestPath claim is invalid")
		}

		// validate operation claim

		if !strings.EqualFold(operationClaim, method) {
			return problems.Forbidden("Access denied", "operation claim does not match the request method")
		}

		// validate requestPath claim
		if !strings.EqualFold(requestPathClaim, opts.LmsBasePath+path) {
			return problems.Forbidden("Access denied", "requestPath claim does not match the request path")
		}

		return nil
	}

	performAdditionalJWTChecks := func(c *fiber.Ctx) error {
		user := c.Locals(UserContextKey).(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)

		// perform LMS Check
		if opts.PerformLMSCheck {
			if err := lmsChecks(claims, c.Method(), c.Path()); err != nil {
				return err
			}
		}

		return nil
	}

	return jwtware.New(jwtware.Config{
		ContextKey: UserContextKey,
		JWKSetURLs: mapIssuerUrls(opts.TrustedIssuers),
		SuccessHandler: func(c *fiber.Ctx) error {
			if err := performAdditionalJWTChecks(c); err != nil {
				return c.Status(fiber.StatusForbidden).JSON(err)
			}
			return c.Next()
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusUnauthorized).JSON(problems.Unauthorized("Unauthorized", "Invalid token"))
		},
	})
}
