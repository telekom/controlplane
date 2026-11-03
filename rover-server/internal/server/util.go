// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// defaultPageLimit matches the default of the shared OpenAPI Limit parameter.
const defaultPageLimit = "20"

// linkPath uses the authenticated JWT's requestPath, or the incoming path otherwise.
func linkPath(c *fiber.Ctx) string {
	if path, ok := security.AuthenticatedRequestPath(c); ok {
		return path
	}
	return c.Path()
}

// buildPageURL retains the request parameters and encodes the continuation cursor.
func buildPageURL(baseURL, path string, query url.Values, cursor string) string {
	if query.Get("limit") == "" {
		query.Set("limit", defaultPageLimit)
	}
	query.Del("cursor")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return baseURL + path + "?" + query.Encode()
}

func buildCursorUrl(baseURL, path, cursor string) string {
	return fmt.Sprintf("%s?cursor=%s", baseURL+path, cursor)
}
