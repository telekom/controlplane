// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"math"
	"net/url"
	"strconv"
)

// GetPortOrDefaultFromScheme returns the port number from the URL if specified.
// If the URL does not specify a port or if the specified port is invalid, it returns the default port for the URL's scheme (80 for http, 443 for https).
// If the URL has an invalid port (non-numeric or out of range), it falls back to the default port for the scheme.
func GetPortOrDefaultFromScheme(url *url.URL) int32 {
	port, err := strconv.Atoi(url.Port())
	if err == nil && port > 0 && port <= math.MaxInt32 {
		return int32(port)
	}

	switch url.Scheme {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 80
	}
}
