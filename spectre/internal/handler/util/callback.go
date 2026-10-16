// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"net/url"
)

// CallbackQueryParam is the query parameter used to pass the original callback
// target through the Gateway's dynamic-upstream feature.
const CallbackQueryParam = "callback"

// BuildGatewayCallbackURL constructs a gateway-mediated callback URL by parsing
// gatewayURL, adding the target callback as the "callback" query parameter
// (preserving any existing query parameters on gatewayURL), and returning the
// assembled URL string.
//
// Both gatewayURL and target must be absolute HTTP(S) URLs; the function returns
// an error otherwise.
func BuildGatewayCallbackURL(gatewayURL, target string) (string, error) {
	gw, err := url.Parse(gatewayURL)
	if err != nil {
		return "", fmt.Errorf("invalid gateway callback URL %q: %w", gatewayURL, err)
	}
	if gw.Scheme != "http" && gw.Scheme != "https" {
		return "", fmt.Errorf("gateway callback URL %q must use http or https scheme", gatewayURL)
	}
	if gw.Host == "" {
		return "", fmt.Errorf("gateway callback URL %q has no host", gatewayURL)
	}

	t, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid callback target %q: %w", target, err)
	}
	if t.Scheme != "http" && t.Scheme != "https" {
		return "", fmt.Errorf("callback target %q must use http or https scheme", target)
	}
	if t.Host == "" {
		return "", fmt.Errorf("callback target %q has no host", target)
	}

	q := gw.Query()
	q.Set(CallbackQueryParam, target)
	gw.RawQuery = q.Encode()

	return gw.String(), nil
}
