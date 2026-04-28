// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// newTestTokenServer creates a test OAuth2 token endpoint.
// grantCount tracks how many password grants were issued.
// failRefresh controls whether refresh grants return invalid_grant.
func newTestTokenServer(grantCount *atomic.Int32, failRefresh *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grantType := r.FormValue("grant_type")

		if grantType == "refresh_token" && failRefresh.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": "Token is not active",
			})
			return
		}

		if grantType == "password" {
			grantCount.Add(1)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "test-refresh-token",
		})
	}))
}

func TestReAuthTokenSource_UsesInnerTokenWhenValid(t *testing.T) {
	validToken := &oauth2.Token{
		AccessToken:  "valid",
		TokenType:    "Bearer",
		RefreshToken: "refresh",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	var grantCount atomic.Int32
	var failRefresh atomic.Bool
	srv := newTestTokenServer(&grantCount, &failRefresh)
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", validToken)
	tok, err := ts.Token()

	require.NoError(t, err)
	assert.Equal(t, "valid", tok.AccessToken)
	// No password grant should have been needed — token was still valid.
	assert.Equal(t, int32(0), grantCount.Load())
}

func TestReAuthTokenSource_ReAuthenticatesOnRefreshFailure(t *testing.T) {
	// Start with an expired token so the inner source will try to refresh.
	expiredToken := &oauth2.Token{
		AccessToken:  "expired",
		TokenType:    "Bearer",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	var grantCount atomic.Int32
	var failRefresh atomic.Bool
	failRefresh.Store(true) // Make refresh fail with invalid_grant (400).

	srv := newTestTokenServer(&grantCount, &failRefresh)
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", expiredToken)
	tok, err := ts.Token()

	require.NoError(t, err)
	assert.Equal(t, "test-access-token", tok.AccessToken)
	// Exactly one password grant for re-authentication.
	assert.Equal(t, int32(1), grantCount.Load())
}

func TestReAuthTokenSource_PropagatesBlockedErrorWhenReAuthFails(t *testing.T) {
	expiredToken := &oauth2.Token{
		AccessToken:  "expired",
		TokenType:    "Bearer",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	// Server that rejects everything with 400.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Token is not active",
		})
	}))
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", expiredToken)
	_, err := ts.Token()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-authentication failed")

	// The error must be classified as blocked (bad credentials won't heal).
	var ae *apiError
	require.True(t, errors.As(err, &ae), "error should be unwrappable to *apiError")
	assert.True(t, ae.IsBlocked(), "bad credentials should be blocked")
	assert.False(t, ae.IsRetryable(), "bad credentials should not be retryable")
}

func TestReAuthTokenSource_TransientErrorSkipsReAuth(t *testing.T) {
	expiredToken := &oauth2.Token{
		AccessToken:  "expired",
		TokenType:    "Bearer",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	var grantCount atomic.Int32

	// Server that returns 503 for refresh attempts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") == "password" {
			grantCount.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "service_unavailable",
		})
	}))
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", expiredToken)
	_, err := ts.Token()

	require.Error(t, err)
	// No password grant should have been attempted — this was a transient error.
	assert.Equal(t, int32(0), grantCount.Load(), "should not attempt re-auth on transient error")

	// The error must be classified as retryable.
	var ae *apiError
	require.True(t, errors.As(err, &ae), "error should be unwrappable to *apiError")
	assert.True(t, ae.IsRetryable(), "server error should be retryable")
	assert.False(t, ae.IsBlocked(), "server error should not be blocked")
}

func TestReAuthTokenSource_ServerErrorOnReAuthIsRetryable(t *testing.T) {
	expiredToken := &oauth2.Token{
		AccessToken:  "expired",
		TokenType:    "Bearer",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	// First request (refresh) returns 400 (invalid_grant) to trigger re-auth.
	// Second request (password grant) returns 500 (transient server error).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grantType := r.FormValue("grant_type")
		w.Header().Set("Content-Type", "application/json")
		if grantType == "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid_grant",
			})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "server_error",
			})
		}
	}))
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", expiredToken)
	_, err := ts.Token()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-authentication failed")

	// Re-auth failed with 500 — should be retryable, not blocked.
	var ae *apiError
	require.True(t, errors.As(err, &ae), "error should be unwrappable to *apiError")
	assert.True(t, ae.IsRetryable(), "server error during re-auth should be retryable")
	assert.False(t, ae.IsBlocked(), "server error during re-auth should not be blocked")
}

func TestReAuthTokenSource_SubsequentCallsUseFreshToken(t *testing.T) {
	expiredToken := &oauth2.Token{
		AccessToken:  "expired",
		TokenType:    "Bearer",
		RefreshToken: "old-refresh",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}

	var grantCount atomic.Int32
	var failRefresh atomic.Bool
	failRefresh.Store(true)

	srv := newTestTokenServer(&grantCount, &failRefresh)
	defer srv.Close()

	cfg := &oauth2.Config{
		ClientID: "test",
		Endpoint: oauth2.Endpoint{TokenURL: srv.URL},
	}
	ctx := context.Background()

	ts := newReAuthTokenSource(ctx, cfg, "user", "pass", expiredToken)

	// First call: triggers re-auth.
	tok1, err := ts.Token()
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", tok1.AccessToken)
	assert.Equal(t, int32(1), grantCount.Load())

	// Second call: should reuse the fresh token (no additional grant).
	tok2, err := ts.Token()
	require.NoError(t, err)
	fmt.Println("second token:", tok2.AccessToken)
	assert.Equal(t, int32(1), grantCount.Load())
}
