// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// TokenSource provides an OAuth access token, refreshing it automatically
// when expired via the client_credentials grant. It wraps the standard
// golang.org/x/oauth2/clientcredentials package.
type TokenSource struct {
	source oauth2.TokenSource
}

// NewTokenSource creates a TokenSource that fetches tokens from the given
// OAuth token endpoint using client_credentials grant. Token caching and
// automatic refresh are handled by the underlying oauth2 library.
func NewTokenSource(tokenURL, clientID, clientSecret string) *TokenSource {
	cfg := &clientcredentials.Config{
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	return &TokenSource{
		source: cfg.TokenSource(context.Background()),
	}
}

// Token returns a valid access token, fetching a new one if needed.
func (ts *TokenSource) Token() (string, error) {
	tok, err := ts.source.Token()
	if err != nil {
		return "", fmt.Errorf("fetching OAuth token: %w", err)
	}
	return tok.AccessToken, nil
}
