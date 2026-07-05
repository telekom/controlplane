// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Obfuscate is response middleware that strips sensitive fields (clientSecret, teamToken)
// from JSON responses when the caller only has "obfuscated" scopes.
//
// Scope logic:
//   - tardis:admin:all, tardis:admin:read, tardis:hub:all, tardis:team:all → full visibility
//   - tardis:admin:obfuscated, tardis:hub:obfuscated → sensitive fields stripped
//
// If ANY non-obfuscated scope is present, the response is returned unmodified.
func Obfuscate() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		if !shouldObfuscate(c) {
			return nil
		}

		contentType := string(c.Response().Header.ContentType())
		if !strings.Contains(contentType, "application/json") {
			return nil
		}

		body := c.Response().Body()
		if len(body) == 0 {
			return nil
		}

		redacted := redactSensitiveFields(body)
		if redacted != nil {
			c.Response().SetBody(redacted)
		}

		return nil
	}
}

// shouldObfuscate returns true if the caller's scopes indicate obfuscation is required.
func shouldObfuscate(c *fiber.Ctx) bool {
	id := ConsumerIdentityFromContext(c)
	if id == nil || len(id.Scopes) == 0 {
		// No identity or no scopes — obfuscate by default (principle of least privilege).
		return true
	}

	for _, scope := range id.Scopes {
		if isFullAccessScope(scope) {
			return false
		}
	}
	return true
}

// isFullAccessScope returns true for scopes that grant full (non-obfuscated) access.
func isFullAccessScope(scope string) bool {
	switch scope {
	case "tardis:admin:all", "tardis:admin:read",
		"tardis:hub:all", "tardis:team:all":
		return true
	}
	return false
}

// redactSensitiveFields removes clientSecret and teamToken from JSON responses.
// Handles both single objects and arrays (paginated list responses).
func redactSensitiveFields(body []byte) []byte {
	// Try as single object first.
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err == nil {
		if redactObject(obj) {
			result, _ := json.Marshal(obj)
			return result
		}
		return nil
	}

	// Try as array.
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err == nil {
		changed := false
		for _, item := range arr {
			if redactObject(item) {
				changed = true
			}
		}
		if changed {
			result, _ := json.Marshal(arr)
			return result
		}
	}

	return nil
}

// redactObject removes sensitive fields from a single JSON object.
// Also handles nested "items" array (paginated responses).
// Returns true if any field was redacted.
func redactObject(obj map[string]any) bool {
	changed := false

	if _, ok := obj["clientSecret"]; ok {
		delete(obj, "clientSecret")
		changed = true
	}
	if _, ok := obj["teamToken"]; ok {
		delete(obj, "teamToken")
		changed = true
	}

	// Handle paginated responses with "items" array.
	if items, ok := obj["items"]; ok {
		if arr, ok := items.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					if redactObject(m) {
						changed = true
					}
				}
			}
		}
	}

	return changed
}
