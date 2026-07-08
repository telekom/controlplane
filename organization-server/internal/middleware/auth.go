// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"strings"

	"github.com/go-logr/logr"
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
)

type contextKey string

const (
	consumerIdentityKey contextKey = "consumerIdentity"
)

// ConsumerIdentity represents the caller's identity extracted from their JWT.
type ConsumerIdentity struct {
	Group  string
	Team   string
	Scopes []string
}

// ConsumerIdentityFromContext retrieves the ConsumerIdentity stored in the request context.
func ConsumerIdentityFromContext(c *fiber.Ctx) *ConsumerIdentity {
	v := c.Locals(string(consumerIdentityKey))
	if v == nil {
		return nil
	}
	id, ok := v.(*ConsumerIdentity)
	if !ok {
		return nil
	}
	return id
}

// JWTValidation creates a middleware that validates JWT tokens.
// When trustedIssuers is non-empty, tokens are fully verified (signature + expiry).
// When trustedIssuers is empty (mock mode), tokens are parsed without verification
// but still decoded and checked for structure.
func JWTValidation(log logr.Logger, trustedIssuers []string) fiber.Handler {
	if len(trustedIssuers) == 0 {
		log.Info("⚠️ Security: mock mode (no trusted issuers configured)")
		return mock.NewJWTMock()
	}

	log.Info("🔑 Security: JWT validation enabled", "issuers", trustedIssuers)
	jwkURLs := make([]string, 0, len(trustedIssuers))
	for _, iss := range trustedIssuers {
		jwkURLs = append(jwkURLs, iss+"/protocol/openid-connect/certs")
	}

	return jwtware.New(jwtware.Config{
		ContextKey: "user",
		JWKSetURLs: jwkURLs,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.V(1).Info("JWT validation failed", "error", err)
			p := problems.Unauthorized("Unauthorized", "Invalid or expired token")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		},
	})
}

// IdentityExtraction creates a middleware that extracts ConsumerIdentity from
// the validated JWT token's claims. Must run after JWTValidation.
//
// It reads clientId (format: "group--team--user") and scope/scopes claims.
func IdentityExtraction(log logr.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(*jwt.Token)
		if !ok || user == nil {
			p := problems.Unauthorized("Unauthorized", "No token in context")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		claims, ok := user.Claims.(jwt.MapClaims)
		if !ok {
			p := problems.Unauthorized("Unauthorized", "Invalid token claims")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		identity, err := extractIdentity(claims)
		if err != nil {
			log.V(1).Info("Failed to extract identity from token", "error", err)
			p := problems.Unauthorized("Unauthorized", "Unable to extract identity from token")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		c.Locals(string(consumerIdentityKey), identity)
		return c.Next()
	}
}

// extractIdentity parses the ConsumerIdentity from JWT claims.
// clientId format: "group--team--user" (e.g. "eni--hyper--team-user")
func extractIdentity(claims jwt.MapClaims) (*ConsumerIdentity, error) {
	clientID, ok := claims["clientId"].(string)
	if !ok || clientID == "" {
		// Fallback: some issuers use "azp" (authorized party)
		clientID, ok = claims["azp"].(string)
		if !ok {
			clientID = ""
		}
	}
	if clientID == "" {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "missing clientId claim")
	}

	parts := strings.Split(clientID, "--")
	if len(parts) < 3 {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "clientId format invalid, expected group--team--user")
	}

	group := parts[0]
	team := parts[1]

	// Parse scopes: try "scopes" (plural, common-server format) then "scope" (Keycloak)
	var scopes []string
	if s, ok := claims["scopes"].(string); ok && s != "" {
		scopes = strings.Fields(s)
	} else if s, ok := claims["scope"].(string); ok && s != "" {
		scopes = strings.Fields(s)
	}

	return &ConsumerIdentity{
		Group:  group,
		Team:   team,
		Scopes: scopes,
	}, nil
}

// TeamAuthorization creates a middleware that verifies the caller's identity
// matches the team they're trying to access (from :hub and :team path params).
// This prevents cross-team access (e.g. team A accessing team B's data).
func TeamAuthorization(log logr.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		identity := ConsumerIdentityFromContext(c)
		if identity == nil {
			p := problems.Forbidden("Forbidden", "No identity in context")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		hub := c.Params("hub")
		team := c.Params("team")

		// If path has :hub, verify it matches the caller's group
		if hub != "" && !strings.EqualFold(identity.Group, hub) {
			log.V(1).Info("Cross-hub access denied",
				"callerGroup", identity.Group,
				"requestedHub", hub,
			)
			p := problems.Forbidden("Forbidden", "Access denied: token does not match requested hub")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		// If path has :team, verify it matches the caller's team
		if team != "" && !strings.EqualFold(identity.Team, team) {
			log.V(1).Info("Cross-team access denied",
				"callerTeam", identity.Team,
				"requestedTeam", team,
			)
			p := problems.Forbidden("Forbidden", "Access denied: token does not match requested team")
			return c.Status(p.Code()).JSON(p, "application/problem+json")
		}

		return c.Next()
	}
}
