// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
	commonclient "github.com/telekom/controlplane/common-server/pkg/client"
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
// caFilePath is the path to the CA bundle for TLS verification (e.g.
// "/var/run/secrets/trust-bundle/trust-bundle.pem"). If empty, system CAs are used.
func NewCPAPIClient(endpoint string, tokenSource *TokenSource, caFilePath string) graphql.Client {
	baseClient := commonclient.NewBaseHttpClient(
		commonclient.WithCaFilepath(caFilePath),
		commonclient.WithClientName("cpapi"),
		commonclient.WithClientTimeout(15*time.Second),
	)

	transport := &cpAPITransport{
		tokenSource: tokenSource,
		base:        baseClient.Transport,
	}
	baseClient.Transport = transport
	return graphql.NewClient(endpoint, baseClient)
}

// cpAPITransport is an http.RoundTripper that injects:
// 1. Bearer token from TokenSource (facade's admin credentials)
// 2. X-Forwarded-* headers from context (consumer identity)
type cpAPITransport struct {
	tokenSource *TokenSource
	base        http.RoundTripper
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

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
