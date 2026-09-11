// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"net/http"

	accesstoken "github.com/telekom/controlplane/common-server/pkg/client/token"
)

// tokenTransport injects a projected ServiceAccount token as the bearer
// credential on every request. The kubelet rotates the token file; the
// FileAccessToken re-reads it when the cached token nears expiry.
type tokenTransport struct {
	token accesstoken.AccessToken
	base  http.RoundTripper
	// decorate runs after the token header is set, so a caller can add its own
	// per-request headers (e.g. forwarded identity). May be nil.
	decorate func(*http.Request)
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != nil {
		tok, err := t.token.Read()
		if err != nil {
			return nil, fmt.Errorf("reading access token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	if t.decorate != nil {
		t.decorate(req)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
