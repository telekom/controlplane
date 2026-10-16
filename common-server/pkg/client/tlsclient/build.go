// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/telekom/controlplane/common-server/pkg/client/metrics"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Client is an HTTP client with its own transport and token source.
type Client struct {
	doer      metrics.HttpRequestDoer
	base      *http.Client
	apiURL    string
	transport *http.Transport
	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    atomic.Bool
}

// Doer returns the request doer to use for API requests.
// Requests should carry the caller's context.
func (c *Client) Doer() metrics.HttpRequestDoer {
	return c.doer
}

// UnauthenticatedDoer shares the client's TLS trust and timeout without adding
// OAuth credentials. Its lifetime is bounded by the same client or lease.
func (c *Client) UnauthenticatedDoer() *http.Client {
	return c.base
}

// APIURL returns the API base URL of the Spec the client was built for.
func (c *Client) APIURL() string {
	return c.apiURL
}

// Close stops the token source and closes idle connections. Clients obtained
// from a Factory are closed by the factory and must not be closed by callers.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.cancel()
		c.transport.CloseIdleConnections()
	})
}

// Option configures New.
type Option func(*buildOptions)

type buildOptions struct {
	insecureSkipVerify bool
}

// WithInsecureSkipVerify disables TLS verification for token and API requests.
// Only available for standalone clients.
func WithInsecureSkipVerify(skip bool) Option {
	return func(o *buildOptions) {
		o.insecureSkipVerify = skip
	}
}

// New builds an uncached client. ctx is used to resolve the CA and as
// lifetime of the token source; the caller must call Close.
// Spec.Owner and Spec.Profile are ignored.
func New(ctx context.Context, profile Profile, spec Spec, opts ...Option) (*Client, error) {
	if err := profile.validate(); err != nil {
		return nil, err
	}
	if err := spec.validate(); err != nil {
		return nil, err
	}
	o := &buildOptions{}
	for _, opt := range opts {
		opt(o)
	}
	caPEM, err := resolveCA(ctx, spec.CA)
	if err != nil {
		return nil, err
	}
	return build(ctx, profile, spec, caPEM, o.insecureSkipVerify)
}

func resolveCA(ctx context.Context, r CAResolver) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	caPEM, err := r.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving CA bundle: %w", err)
	}
	return caPEM, nil
}

func newTransport(p Profile, caPEM []byte, insecure bool) (*http.Transport, error) {
	pool, err := NewCertPool(caPEM)
	if err != nil {
		return nil, err
	}
	var t *http.Transport
	if p.Transport == TransportDefault {
		t = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		t = &http.Transport{}
	}
	t.TLSClientConfig = &tls.Config{
		RootCAs:            pool,
		MinVersion:         p.MinTLSVersion,
		InsecureSkipVerify: insecure, //nolint:gosec // explicit opt-in for standalone tools only
	}
	if p.MaxIdleConnsPerHost > 0 {
		t.MaxIdleConnsPerHost = p.MaxIdleConnsPerHost
	}
	return t, nil
}

// build creates the client. lifeCtx bounds the token source lifetime and must
// not be a reconciliation context.
func build(lifeCtx context.Context, p Profile, s Spec, caPEM []byte, insecure bool) (*Client, error) {
	transport, err := newTransport(p, caPEM, insecure)
	if err != nil {
		return nil, err
	}
	base := &http.Client{Transport: transport, Timeout: p.timeout()}

	ctx, cancel := context.WithCancel(lifeCtx)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, base)
	c := &Client{apiURL: s.APIURL, base: base, transport: transport, cancel: cancel}

	var httpClient *http.Client
	switch s.Auth.Mode {
	case AuthClientCredentials:
		cfg := &clientcredentials.Config{
			ClientID:     s.Auth.ClientID,
			ClientSecret: s.Auth.ClientSecret,
			TokenURL:     s.Auth.TokenURL,
		}
		httpClient = cfg.Client(ctx)
	case AuthPassword:
		cfg := &oauth2.Config{
			ClientID:     s.Auth.ClientID,
			ClientSecret: s.Auth.ClientSecret,
			Endpoint:     oauth2.Endpoint{TokenURL: s.Auth.TokenURL},
		}
		token, err := cfg.PasswordCredentialsToken(ctx, s.Auth.Username, s.Auth.Password)
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("retrieving token: %w", err)
		}
		src := newReAuthTokenSource(ctx, cfg, s.Auth.Username, s.Auth.Password, token, p.ClassifyTokenError)
		httpClient = oauth2.NewClient(ctx, src)
	default:
		httpClient = base
	}

	c.doer = httpClient
	if p.MetricsClientName != "" {
		c.doer = metrics.WithMetrics(httpClient,
			metrics.WithClientName(p.MetricsClientName),
			metrics.WithReplacePatterns(p.MetricsReplacePatterns...),
		)
	}
	return c, nil
}
