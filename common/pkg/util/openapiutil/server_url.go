// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package openapiutil contains helpers for working with OpenAPI documents.
package openapiutil

import (
	"fmt"
	"strings"
)

// PathFromURL extracts the path of an OpenAPI server URL without normalizing it.
// Query and fragment are ignored, scheme and authority are skipped (they may contain
// undeclared templates like {host}), and percent-encodings are kept unchanged.
// It returns "" if the URL has an authority but no path.
// The path must not contain template variables and must be valid RFC 3986 path syntax.
func PathFromURL(rawURL string) (string, error) {
	path := rawURL
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	// A colon before the first slash ends the scheme (RFC 3986 forbids it in a relative path's first segment).
	if i := strings.IndexAny(path, ":/"); i > 0 && path[i] == ':' {
		path = path[i+1:]
	}
	if strings.HasPrefix(path, "//") {
		i := strings.Index(path[2:], "/")
		if i < 0 {
			return "", nil
		}
		path = path[2+i:]
	}
	if strings.ContainsAny(path, "{}") {
		return "", fmt.Errorf("server url path %q must not contain variables", path)
	}
	if err := validateURLPath(path); err != nil {
		return "", fmt.Errorf("invalid server url path %q: %w", path, err)
	}
	return path, nil
}

// validateURLPath checks RFC 3986 path syntax: "/" and pchar
// (unreserved, sub-delims, ":", "@" and well-formed percent-encodings).
func validateURLPath(path string) error {
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case strings.IndexByte("/-._~!$&'()*+,;=:@", c) >= 0:
		case c == '%':
			if i+2 >= len(path) || !isHex(path[i+1]) || !isHex(path[i+2]) {
				return fmt.Errorf("malformed percent-encoding at position %d", i)
			}
			i += 2
		default:
			return fmt.Errorf("invalid character %q at position %d", c, i)
		}
	}
	return nil
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
