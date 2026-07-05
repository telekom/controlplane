// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
)

// contextKey for storing consumer identity in context.
type contextKey string

const identityContextKey contextKey = "consumerIdentity"

// ConsumerIdentity holds the forwarded consumer context for CP API calls.
type ConsumerIdentity struct {
	Environment string
	Group       string
	Team        string
}

// WithIdentity adds a ConsumerIdentity to the context for use by the transport.
func WithIdentity(ctx context.Context, id *ConsumerIdentity) context.Context {
	return context.WithValue(ctx, identityContextKey, id)
}

// identityFromContext extracts ConsumerIdentity from context.
func identityFromContext(ctx context.Context) *ConsumerIdentity {
	v, _ := ctx.Value(identityContextKey).(*ConsumerIdentity)
	return v
}

// NewCPAPIClient creates a genqlient GraphQL client that authenticates to
// CP API as admin (via TokenSource) and forwards consumer identity via
// X-Forwarded-* headers (extracted from context).
func NewCPAPIClient(endpoint string, tokenSource *TokenSource) graphql.Client {
	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: &cpAPITransport{tokenSource: tokenSource},
	}
	return graphql.NewClient(endpoint, httpClient)
}

// cpAPITransport is an http.RoundTripper that injects:
// 1. Bearer token from TokenSource (facade's admin credentials)
// 2. X-Forwarded-* headers from context (consumer identity)
type cpAPITransport struct {
	tokenSource *TokenSource
}

func (t *cpAPITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Inject admin token.
	if t.tokenSource != nil {
		token, err := t.tokenSource.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Inject consumer identity from context.
	if id := identityFromContext(req.Context()); id != nil {
		if id.Environment != "" {
			req.Header.Set("X-Forwarded-Environment", id.Environment)
		}
		if id.Group != "" {
			req.Header.Set("X-Forwarded-Group", id.Group)
		}
		if id.Team != "" {
			req.Header.Set("X-Forwarded-Team", id.Team)
		}
	}

	return http.DefaultTransport.RoundTrip(req)
}
