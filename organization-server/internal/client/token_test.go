// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestTokenSource_FetchesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	ts := NewTokenSource(srv.URL, "client-id", "client-secret")

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "test-token" {
		t.Fatalf("expected 'test-token', got %q", tok)
	}
}

func TestTokenSource_CachesToken(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "cached-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	ts := NewTokenSource(srv.URL, "client-id", "client-secret")

	// First call fetches a new token.
	_, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should use cached token (no additional HTTP call).
	tok2, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok2 != "cached-token" {
		t.Fatalf("expected 'cached-token', got %q", tok2)
	}

	if callCount.Load() != 1 {
		t.Fatalf("expected 1 HTTP call (cached), got %d", callCount.Load())
	}
}

func TestTokenSource_HandlesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ts := NewTokenSource(srv.URL, "client-id", "client-secret")

	_, err := ts.Token()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
