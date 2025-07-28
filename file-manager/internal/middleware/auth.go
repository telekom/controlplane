// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"strings"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
)

// BearerAuthMiddleware extracts the bearer token from the Authorization header
// and adds it to the context for use by the minio client
func BearerAuthMiddleware(log logr.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract the Authorization header
		authHeader := c.Get("Authorization")

		// Check if the header is present and has the bearer token format
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			// Extract the token (remove "Bearer " prefix)
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Add token to context
			ctx := WithBearerToken(c.Context(), token)
			c.SetUserContext(ctx)

			log.V(1).Info("Bearer token extracted and added to context")
		} else {
			log.V(1).Info("No valid bearer token found in Authorization header")
		}

		// Continue with the next handler
		return c.Next()
	}
}
