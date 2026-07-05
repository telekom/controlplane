// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
)

type contextKey string

const (
	consumerIdentityKey contextKey = "consumerIdentity"
	rawTokenKey         contextKey = "rawToken"
)

// ConsumerIdentity represents the caller's identity extracted from their JWT.
type ConsumerIdentity struct {
	Environment string
	Group       string
	Team        string
	Scopes      []string
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

// RawTokenFromContext returns the raw Bearer token from the request.
func RawTokenFromContext(c *fiber.Ctx) string {
	v := c.Locals(string(rawTokenKey))
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// TokenDecode is middleware that decodes (NOT verifies) the consumer's JWT
// and stores the identity in the request context. This is safe because:
// 1. The gateway/ingress already verified the token signature
// 2. We only need the claims for routing/identity forwarding
func TokenDecode(log logr.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": 401,
				"detail": "Missing or invalid Authorization header",
			})
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		c.Locals(string(rawTokenKey), token)

		identity, err := decodeToken(token)
		if err != nil {
			log.V(1).Info("Failed to decode consumer token", "error", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"type":   "about:blank",
				"title":  "Unauthorized",
				"status": 401,
				"detail": "Unable to decode token claims",
			})
		}

		c.Locals(string(consumerIdentityKey), identity)
		return c.Next()
	}
}

// decodeToken extracts claims from a JWT without verifying the signature.
func decodeToken(token string) (*ConsumerIdentity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "invalid token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims struct {
		Env   string `json:"env"`
		Group string `json:"group"`
		Team  string `json:"team"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}

	var scopes []string
	if claims.Scope != "" {
		scopes = strings.Fields(claims.Scope)
	}

	return &ConsumerIdentity{
		Environment: claims.Env,
		Group:       claims.Group,
		Team:        claims.Team,
		Scopes:      scopes,
	}, nil
}
