// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// NewTokenSource creates an oauth2.TokenSource that fetches tokens from the
// given OAuth token endpoint using client_credentials grant. Token caching and
// automatic refresh are handled by the underlying oauth2 library.
func NewTokenSource(tokenURL, clientID, clientSecret string) oauth2.TokenSource {
	cfg := &clientcredentials.Config{
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	return cfg.TokenSource(context.Background())
}
