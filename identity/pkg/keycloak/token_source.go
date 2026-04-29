// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

// reAuthTokenSource wraps an oauth2.TokenSource and falls back to a fresh
// Resource Owner Password Credentials grant when the inner source fails
// because the refresh token is invalid (HTTP 400/401). Transient errors
// (network, 5xx) are not retried with a password grant and instead
// propagated as classified apiErrors for proper controller handling.
type reAuthTokenSource struct {
	mu       sync.Mutex
	cfg      *oauth2.Config
	ctx      context.Context
	username string
	password string
	inner    oauth2.TokenSource
}

// newReAuthTokenSource creates a TokenSource that automatically
// re-authenticates with username/password when token refresh fails
// due to an invalid refresh token.
func newReAuthTokenSource(ctx context.Context, cfg *oauth2.Config, username, password string, initial *oauth2.Token) oauth2.TokenSource {
	return &reAuthTokenSource{
		cfg:      cfg,
		ctx:      ctx,
		username: username,
		password: password,
		inner:    cfg.TokenSource(ctx, initial),
	}
}

func (s *reAuthTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.inner.Token()
	if err == nil {
		return token, nil
	}

	// Only re-authenticate when the refresh token is actually invalid
	// (400/401). For transient errors (network, 5xx, 429) propagate a
	// classified error so the controller can apply correct retry logic.
	if !isTokenInvalid(err) {
		return nil, fmt.Errorf("token refresh failed: %w", classifyOAuth2Error(err))
	}

	// Refresh token is invalid — attempt full re-authentication.
	token, reAuthErr := s.cfg.PasswordCredentialsToken(s.ctx, s.username, s.password)
	if reAuthErr != nil {
		return nil, fmt.Errorf("re-authentication failed: %w", classifyOAuth2Error(reAuthErr))
	}

	// Replace inner source so subsequent calls use the new token.
	s.inner = s.cfg.TokenSource(s.ctx, token)
	return token, nil
}

// isTokenInvalid returns true when the error indicates the refresh token
// itself is invalid (HTTP 400 or 401 from the token endpoint), which is
// the only situation where a full password-grant re-authentication can help.
func isTokenInvalid(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		return re.Response.StatusCode == http.StatusBadRequest ||
			re.Response.StatusCode == http.StatusUnauthorized
	}
	return false
}

// classifyOAuth2Error converts an oauth2 token-endpoint error into an
// apiError with the correct retry/blocked classification. This ensures
// errors from the token layer integrate with ctrlerrors.HandleError via
// the same duck-typed interfaces as Keycloak API errors.
func classifyOAuth2Error(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		apiErr := CheckHTTPStatus(re.Response.StatusCode)
		if apiErr != nil {
			return apiErr
		}
	}

	// Network, DNS, timeout, or other non-HTTP errors — retryable.
	return &apiError{
		message:      fmt.Sprintf("token endpoint error: %s", err.Error()),
		retryAllowed: true,
	}
}
