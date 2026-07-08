// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenSource provides an OAuth access token, refreshing it automatically
// when expired via the client_credentials grant.
type TokenSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewTokenSource creates a TokenSource that fetches tokens from the given
// OAuth token endpoint using client_credentials grant.
func NewTokenSource(tokenURL, clientID, clientSecret string) *TokenSource {
	return &TokenSource{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Token returns a valid access token, fetching a new one if needed.
func (ts *TokenSource) Token() (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Return cached token if still valid (with 30s buffer).
	if ts.token != "" && time.Now().Before(ts.expiry.Add(-30*time.Second)) {
		return ts.token, nil
	}

	token, expiry, err := ts.fetchToken()
	if err != nil {
		return "", err
	}
	ts.token = token
	ts.expiry = expiry
	return ts.token, nil
}

func (ts *TokenSource) fetchToken() (string, time.Time, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {ts.clientID},
		"client_secret": {ts.clientSecret},
	}

	resp, err := ts.httpClient.Post(ts.tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode())) //nolint:gosec // tokenURL is from trusted config, not user input
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fetching OAuth token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("OAuth token endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decoding token response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return result.AccessToken, expiry, nil
}
