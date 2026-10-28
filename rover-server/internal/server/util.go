// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/url"
	"strconv"
)

// defaultPageLimit matches the default of the shared OpenAPI Limit parameter.
const defaultPageLimit = 20

// buildPageURL retains the request parameters and encodes the continuation cursor.
func buildPageURL(baseURL, path string, query url.Values, cursor string) string {
	if query.Get("limit") == "" {
		query.Set("limit", strconv.Itoa(defaultPageLimit))
	}
	query.Del("cursor")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return baseURL + path + "?" + query.Encode()
}

func buildCursorUrl(baseURL string, path string, cursor string) string {
	return fmt.Sprintf("%s?cursor=%s", baseURL+path, cursor)
}
