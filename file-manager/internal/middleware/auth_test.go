// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
)

func TestBearerAuthMiddleware(t *testing.T) {
	// Create a test Fiber app
	app := fiber.New()

	// Add the middleware
	app.Use(BearerAuthMiddleware(logr.Discard()))

	// Add a test handler to verify the token was added to the context
	app.Get("/test", func(c *fiber.Ctx) error {
		token, err := ExtractBearerTokenFromContext(c.UserContext())
		if err != nil {
			return c.SendStatus(500)
		}
		return c.SendString(token)
	})

	// Test case 1: Request with valid Bearer token
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Error testing request: %v", err)
	}

	// Check response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// Read body
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	if body != "test-token-123" {
		t.Errorf("Expected body test-token-123, got %s", body)
	}

	// Test case 2: Request without Authorization header
	req = httptest.NewRequest(http.MethodGet, "/test", nil)

	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("Error testing request: %v", err)
	}

	// Expect a 500 because the token couldn't be extracted
	if resp.StatusCode != 500 {
		t.Errorf("Expected status code 500, got %d", resp.StatusCode)
	}
}
