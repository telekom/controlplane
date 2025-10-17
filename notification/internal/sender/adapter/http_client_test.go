// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

func TestNewRestyClient_DefaultConfig(t *testing.T) {
	client := NewRestyClient(nil)

	assertNotNil(t, client, "NewRestyClient()")

	// Verify client is properly initialized with local test server
	server := newSuccessServer(t, http.StatusOK, "")
	defer server.Close()

	resp, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request")
	assertStatusCode(t, resp, 200)
}

func TestNewRestyClient_WithConfig(t *testing.T) {
	config := &HTTPClientConfig{
		Timeout:             10 * time.Second,
		MaxIdleConns:        50,
		MaxConnsPerHost:     5,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxRetries:          2,
		RetryInitialBackoff: 500 * time.Millisecond,
		RetryMaxBackoff:     5 * time.Second,
		UserAgent:           "TestClient/1.0",
		Debug:               false,
	}

	client := NewRestyClient(config)

	assertNotNil(t, client, "NewRestyClient()")

	// Verify User-Agent is set
	headers := client.Header
	if userAgent := headers.Get("User-Agent"); userAgent != "TestClient/1.0" {
		t.Errorf("Expected User-Agent 'TestClient/1.0', got '%s'", userAgent)
	}
}

func TestNewRestyClient_EmptyConfig(t *testing.T) {
	config := &HTTPClientConfig{}

	client := NewRestyClient(config)

	assertNotNil(t, client, "NewRestyClient() with empty config")

	// Should have default values applied
	// We can't directly inspect timeout, but we can verify client works
	server := newSuccessServer(t, http.StatusOK, "")
	defer server.Close()

	resp, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request")
	assertStatusCode(t, resp, 200)
}

func TestNewRestyClient_DefaultsApplied(t *testing.T) {
	tests := []struct {
		name   string
		config *HTTPClientConfig
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name:   "empty config",
			config: &HTTPClientConfig{},
		},
		{
			name: "partial config - only timeout",
			config: &HTTPClientConfig{
				Timeout: 5 * time.Second,
			},
		},
		{
			name: "partial config - only user agent",
			config: &HTTPClientConfig{
				UserAgent: "CustomAgent/2.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewRestyClient(tt.config)

			assertNotNil(t, client, "NewRestyClient()")

			// All clients should work regardless of config
			server := newSuccessServer(t, http.StatusOK, "")
			defer server.Close()

			resp, err := client.R().Get(server.URL)
			assertNoError(t, err, "Request")
			assertStatusCode(t, resp, 200)
		})
	}
}

func TestNewRestyClient_WithRetryCondition(t *testing.T) {
	// Custom retry condition that only retries on 503
	customRetryCondition := func(r *resty.Response, err error) bool {
		if err != nil {
			return false
		}
		return r.StatusCode() == 503
	}

	server, attemptCount := newRetryServer(t, http.StatusServiceUnavailable, http.StatusOK, 2)
	defer server.Close()

	config := &HTTPClientConfig{
		MaxRetries:          3,
		RetryInitialBackoff: 10 * time.Millisecond,
		RetryMaxBackoff:     50 * time.Millisecond,
		RetryConditionFunc:  customRetryCondition,
	}

	client := NewRestyClient(config)

	resp, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request")
	assertStatusCode(t, resp, 200)
	assertAttempts(t, *attemptCount, 3, "Custom retry condition")
}

func TestNewRestyClient_WithHooks(t *testing.T) {
	beforeRequestCalled := false
	afterResponseCalled := false

	config := &HTTPClientConfig{
		OnBeforeRequest: func(c *resty.Client, req *resty.Request) error {
			beforeRequestCalled = true
			return nil
		},
		OnAfterResponse: func(c *resty.Client, resp *resty.Response) error {
			afterResponseCalled = true
			return nil
		},
	}

	client := NewRestyClient(config)

	// Test successful request (should call before and after hooks)
	server := newSuccessServer(t, http.StatusOK, "")
	defer server.Close()

	_, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request")

	if !beforeRequestCalled {
		t.Error("OnBeforeRequest hook was not called")
	}

	if !afterResponseCalled {
		t.Error("OnAfterResponse hook was not called")
	}
}

func TestNewRestyClient_WithOnErrorHook(t *testing.T) {
	onErrorCalled := false
	var capturedError error

	config := &HTTPClientConfig{
		Timeout:    100 * time.Millisecond,
		MaxRetries: 0, // Disable retries to ensure error hook is called once
		OnError: func(req *resty.Request, err error) {
			onErrorCalled = true
			capturedError = err
		},
	}

	client := NewRestyClient(config)

	// Test with server that times out
	server := newDelayedServer(t, 200*time.Millisecond, http.StatusOK)
	defer server.Close()

	_, _ = client.R().Get(server.URL)

	if !onErrorCalled {
		t.Error("OnError hook was not called for timeout error")
	}

	if capturedError == nil {
		t.Error("Expected error to be captured in OnError hook")
	}
}

func TestNewRestyClient_DebugMode(t *testing.T) {
	config := &HTTPClientConfig{
		Debug: true,
	}

	client := NewRestyClient(config)

	assertNotNil(t, client, "NewRestyClient() with debug mode")

	// Debug mode should be enabled (we can't directly test this,
	// but we can verify the client works)
	server := newSuccessServer(t, http.StatusOK, "")
	defer server.Close()

	_, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request with debug mode")
}

func TestNewRestyClient_TransportConfiguration(t *testing.T) {
	config := &HTTPClientConfig{
		MaxIdleConns:        200,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     120 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}

	client := NewRestyClient(config)

	assertNotNil(t, client, "NewRestyClient()")

	// Verify transport is properly configured
	transport := client.GetClient().Transport.(*http.Transport)

	if transport.MaxIdleConns != 200 {
		t.Errorf("Expected MaxIdleConns 200, got %d", transport.MaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != 20 {
		t.Errorf("Expected MaxIdleConnsPerHost 20, got %d", transport.MaxIdleConnsPerHost)
	}

	if transport.MaxConnsPerHost != 20 {
		t.Errorf("Expected MaxConnsPerHost 20, got %d", transport.MaxConnsPerHost)
	}

	if transport.IdleConnTimeout != 120*time.Second {
		t.Errorf("Expected IdleConnTimeout 120s, got %v", transport.IdleConnTimeout)
	}

	if transport.TLSHandshakeTimeout != 15*time.Second {
		t.Errorf("Expected TLSHandshakeTimeout 15s, got %v", transport.TLSHandshakeTimeout)
	}

	if !transport.ForceAttemptHTTP2 {
		t.Error("Expected ForceAttemptHTTP2 to be true")
	}
}

func TestNewRestyClient_ProxyPreserved(t *testing.T) {
	// Verify that cloning DefaultTransport preserves proxy settings
	client := NewRestyClient(nil)

	assertNotNil(t, client, "NewRestyClient()")

	transport := client.GetClient().Transport.(*http.Transport)

	// Verify Proxy function is set (from DefaultTransport)
	if transport.Proxy == nil {
		t.Error("Expected Proxy function to be preserved from DefaultTransport")
	}
}

func TestDefaultRetryCondition_NetworkError(t *testing.T) {
	// Test retry on network error
	shouldRetry := DefaultRetryCondition(nil, http.ErrServerClosed)

	if !shouldRetry {
		t.Error("Expected retry on network error")
	}
}

func TestDefaultRetryCondition_NilResponseNoError(t *testing.T) {
	// Test edge case: nil response with no error should not retry
	shouldRetry := DefaultRetryCondition(nil, nil)

	if shouldRetry {
		t.Error("Should not retry when both response and error are nil")
	}
}

func TestDefaultRetryCondition_StatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		shouldRetry bool
	}{
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"204 No Content", http.StatusNoContent, false},
		{"301 Moved Permanently", http.StatusMovedPermanently, false},
		{"400 Bad Request", http.StatusBadRequest, false},
		{"401 Unauthorized", http.StatusUnauthorized, false},
		{"403 Forbidden", http.StatusForbidden, false},
		{"404 Not Found", http.StatusNotFound, false},
		{"408 Request Timeout", http.StatusRequestTimeout, true},
		{"429 Too Many Requests", http.StatusTooManyRequests, true},
		{"500 Internal Server Error", http.StatusInternalServerError, true},
		{"501 Not Implemented", http.StatusNotImplemented, true},
		{"502 Bad Gateway", http.StatusBadGateway, true},
		{"503 Service Unavailable", http.StatusServiceUnavailable, true},
		{"504 Gateway Timeout", http.StatusGatewayTimeout, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock response
			mockResp := &resty.Response{}
			mockResp.RawResponse = &http.Response{
				StatusCode: tt.statusCode,
			}

			shouldRetry := DefaultRetryCondition(mockResp, nil)

			if shouldRetry != tt.shouldRetry {
				t.Errorf("Status %d: expected retry=%v, got retry=%v",
					tt.statusCode, tt.shouldRetry, shouldRetry)
			}
		})
	}
}

func TestDefaultRetryCondition_NoError(t *testing.T) {
	// Create mock 200 OK response
	mockResp := &resty.Response{}
	mockResp.RawResponse = &http.Response{
		StatusCode: http.StatusOK,
	}

	shouldRetry := DefaultRetryCondition(mockResp, nil)

	if shouldRetry {
		t.Error("Should not retry on successful 200 response")
	}
}

func TestNewRestyClient_RetryBehavior(t *testing.T) {
	tests := []struct {
		name                  string
		failureStatusCode     int
		failuresBeforeSuccess int
		maxRetries            int
		expectedAttempts      int
		expectedFinalStatus   int
	}{
		{
			name:                  "retry on 500 until success",
			failureStatusCode:     http.StatusInternalServerError,
			failuresBeforeSuccess: 2,
			maxRetries:            3,
			expectedAttempts:      3,
			expectedFinalStatus:   200,
		},
		{
			name:                  "retry on 429 until success",
			failureStatusCode:     http.StatusTooManyRequests,
			failuresBeforeSuccess: 1,
			maxRetries:            2,
			expectedAttempts:      2,
			expectedFinalStatus:   200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, attemptCount := newRetryServer(t, tt.failureStatusCode, http.StatusOK, tt.failuresBeforeSuccess)
			defer server.Close()

			client := NewRestyClient(testRetryConfig(tt.maxRetries))

			resp, err := client.R().Get(server.URL)
			assertNoError(t, err, "Request")
			assertStatusCode(t, resp, tt.expectedFinalStatus)
			assertAttempts(t, *attemptCount, tt.expectedAttempts, tt.name)
		})
	}
}

func TestNewRestyClient_NoRetryOn4xx(t *testing.T) {
	// Use high number for failuresBeforeSuccess to ensure it never succeeds
	server, attemptCount := newRetryServer(t, http.StatusBadRequest, http.StatusOK, 999)
	defer server.Close()

	client := NewRestyClient(testRetryConfig(3))

	resp, err := client.R().Get(server.URL)
	assertNoError(t, err, "Request")
	assertStatusCode(t, resp, 400)

	// Should only attempt once (no retry on 4xx except 408, 429)
	assertAttempts(t, *attemptCount, 1, "No retry on 400")
}

func TestNewRestyClient_HTTPVersions(t *testing.T) {
	client := NewRestyClient(nil)

	assertNotNil(t, client, "NewRestyClient()")

	// Verify HTTP/2 is enabled
	transport := client.GetClient().Transport.(*http.Transport)

	if !transport.ForceAttemptHTTP2 {
		t.Error("Expected HTTP/2 to be enabled")
	}
}

func TestNewRestyClient_TimeoutRespected(t *testing.T) {
	// Create server that delays response
	server := newDelayedServer(t, 200*time.Millisecond, http.StatusOK)
	defer server.Close()

	// Create client with short timeout
	client := NewRestyClient(testTimeoutConfig(50*time.Millisecond, 0))

	_, err := client.R().Get(server.URL)

	// Should time out
	assertError(t, err, "Timeout")
}

func TestNewRestyClient_ConcurrentRequests(t *testing.T) {
	server, requestCount := newCountingServer(t, http.StatusOK, 10*time.Millisecond)
	defer server.Close()

	client := NewRestyClient(&HTTPClientConfig{
		MaxIdleConns:    50,
		MaxConnsPerHost: 10,
	})

	// Make concurrent requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := client.R().Get(server.URL)
			if err != nil {
				t.Errorf("Concurrent request failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	assertAttempts(t, *requestCount, 10, "Concurrent requests")
}
