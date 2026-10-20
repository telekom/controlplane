// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tlsclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
)

// reAuthTokenSource refreshes tokens and falls back to a password grant when
// the refresh is rejected with HTTP 400/401. Other errors are classified and
// returned without re-authentication.
type reAuthTokenSource struct {
	mu       sync.Mutex
	cfg      *oauth2.Config
	ctx      context.Context
	username string
	password string
	inner    oauth2.TokenSource
	classify func(error) error
}

func newReAuthTokenSource(ctx context.Context, cfg *oauth2.Config, username, password string, initial *oauth2.Token, classify func(error) error) oauth2.TokenSource {
	if classify == nil {
		classify = func(err error) error { return err }
	}
	return &reAuthTokenSource{
		cfg:      cfg,
		ctx:      ctx,
		username: username,
		password: password,
		inner:    cfg.TokenSource(ctx, initial),
		classify: classify,
	}
}

func (s *reAuthTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.inner.Token()
	if err == nil {
		return token, nil
	}
	if !isTokenInvalid(err) {
		return nil, fmt.Errorf("token refresh failed: %w", s.classify(err))
	}

	token, err = s.cfg.PasswordCredentialsToken(s.ctx, s.username, s.password)
	if err != nil {
		return nil, fmt.Errorf("re-authentication failed: %w", s.classify(err))
	}
	s.inner = s.cfg.TokenSource(s.ctx, token)
	return token, nil
}

func isTokenInvalid(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) && re.Response != nil {
		return re.Response.StatusCode == http.StatusBadRequest ||
			re.Response.StatusCode == http.StatusUnauthorized
	}
	return false
}
