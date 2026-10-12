// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package urlutil provides URL primitives shared by provider endpoint comparisons and
// custom-trust validation. Callers keep their own acceptance policies and error messages.
package urlutil

import (
	"net"
	"net/url"
	"strings"
)

// IsHTTPS reports whether u uses the https scheme, case-insensitively.
func IsHTTPS(u *url.URL) bool {
	return strings.EqualFold(u.Scheme, "https")
}

// CanonicalHost returns the lowercase host of u for comparison: the port is omitted when it is
// the default of the (case-insensitive) scheme, and IPv6 literals are bracketed.
func CanonicalHost(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}
