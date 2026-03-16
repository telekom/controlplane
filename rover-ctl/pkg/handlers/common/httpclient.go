// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

func newHttpClient() HttpDoer {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 10 * time.Second,
	}
}

var NewAuthorizedHttpClient = func(ctx context.Context, tokenUrl, clientId, clientSecret string) HttpDoer {
	baseClient := newHttpClient()

	tokenCfg := clientcredentials.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		TokenURL:     tokenUrl,
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, baseClient)
	return tokenCfg.Client(ctx)
}

var _ HttpDoer = (*staticHeaderHttpDoer)(nil)

type staticHeaderHttpDoer struct {
	headers     http.Header
	innerClient HttpDoer
}

func (s *staticHeaderHttpDoer) Do(req *http.Request) (*http.Response, error) {
	for key, values := range s.headers {
		// Use Set for the first value to avoid duplicating singleton headers
		// (e.g., Authorization, User-Agent) that may already exist on the request.
		if len(values) > 0 {
			req.Header.Set(key, values[0])
			for _, value := range values[1:] {
				req.Header.Add(key, value)
			}
		}
	}
	return s.innerClient.Do(req)
}

func WithStaticHeaders(client HttpDoer, headers http.Header) HttpDoer {
	return &staticHeaderHttpDoer{
		headers:     headers,
		innerClient: client,
	}
}
