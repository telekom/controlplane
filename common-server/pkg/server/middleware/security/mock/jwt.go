// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

func parseMockToken(rawToken string) (*jwt.Token, error) {
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(rawToken, &claims)
	if err != nil {
		return nil, err
	}

	return &jwt.Token{
		Claims: claims,
	}, nil
}

func getBearerToken(c *fiber.Ctx) (string, error) {
	header := c.Get("Authorization")
	if header == "" {
		return "", fmt.Errorf("authorization header not found")
	}

	parts := strings.Split(header, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("invalid Authorization header")
	}

	return parts[1], nil
}

// NewJWTMock creates a new JWT middleware for mocking
// This middleware will parse the JWT token and set the user context
// However, it will not perform any validation on it
func NewJWTMock() fiber.Handler {
	return func(c *fiber.Ctx) error {

		bearerToken, err := getBearerToken(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(problems.Unauthorized("Invalid token", err.Error()))
		}

		token, err := parseMockToken(bearerToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(problems.Unauthorized("Invalid token", err.Error()))
		}

		c.Locals("user", token)
		return c.Next()
	}
}

// NewMockAccessToken creates a new mock access token
// that can be used for testing
func NewMockAccessToken(env, group, team string, scopes []string) string {
	claims := jwt.MapClaims{
		"env":      env,
		"clientId": fmt.Sprintf("%s--%s--%s", group, team, "user"),
		"scopes":   strings.Join(scopes, " "),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		panic(err)
	}
	return tokenString
}
