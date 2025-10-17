// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
)

// Test server helpers

// newSuccessServer creates a test server that always returns the given status code
func newSuccessServer(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if responseBody != "" {
			_, _ = w.Write([]byte(responseBody))
		}
	}))
}

// newRetryServer creates a test server that fails N times before succeeding
// Returns the server and a pointer to the attempt counter
//
// successCode is always http.StatusOK in current tests, but kept as parameter
// for flexibility and to make test intent explicit
//
//nolint:unparam
func newRetryServer(t *testing.T, failureCode, successCode, failuresBeforeSuccess int) (*httptest.Server, *int) {
	t.Helper()
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount <= failuresBeforeSuccess {
			w.WriteHeader(failureCode)
			return
		}
		w.WriteHeader(successCode)
		_, _ = w.Write([]byte("1"))
	}))
	return server, &attemptCount
}

// newDelayedServer creates a test server that delays response by the given duration
//
// statusCode is currently always http.StatusOK, but kept as parameter for flexibility
// in testing different response scenarios
//
//nolint:unparam
func newDelayedServer(t *testing.T, delay time.Duration, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(statusCode)
	}))
}

// newCountingServer creates a test server that counts requests and returns success
func newCountingServer(t *testing.T, statusCode int, delay time.Duration) (*httptest.Server, *int) {
	t.Helper()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(statusCode)
	}))
	return server, &requestCount
}

// Config builder helpers

// testRetryConfig creates a standard HTTPClientConfig for retry testing
func testRetryConfig(maxRetries int) *HTTPClientConfig {
	return &HTTPClientConfig{
		MaxRetries:          maxRetries,
		RetryInitialBackoff: 10 * time.Millisecond,
		RetryMaxBackoff:     50 * time.Millisecond,
		RetryConditionFunc:  DefaultRetryCondition,
	}
}

// testTimeoutConfig creates an HTTPClientConfig with timeout settings
func testTimeoutConfig(timeout time.Duration, maxRetries int) *HTTPClientConfig {
	return &HTTPClientConfig{
		Timeout:             timeout,
		MaxRetries:          maxRetries,
		RetryInitialBackoff: 10 * time.Millisecond,
		RetryMaxBackoff:     50 * time.Millisecond,
	}
}

// Assertion helpers

// assertNoError fails the test if err is not nil
func assertNoError(t *testing.T, err error, context string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", context, err)
	}
}

// assertError fails the test if err is nil
func assertError(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error, got nil", context)
	}
}

// assertErrorContains fails the test if err is nil or doesn't contain the expected string
func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error containing %q, got nil", expected)
	}
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error containing %q, got %q", expected, err.Error())
	}
}

// assertStatusCode fails the test if the response status code doesn't match expected
func assertStatusCode(t *testing.T, resp *resty.Response, expected int) {
	t.Helper()
	if resp.StatusCode() != expected {
		t.Errorf("Expected status %d, got %d", expected, resp.StatusCode())
	}
}

// assertAttempts fails the test if the attempt count doesn't match expected
func assertAttempts(t *testing.T, actual, expected int, context string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected %d attempts, got %d", context, expected, actual)
	}
}

// assertNotNil fails the test if the value is nil
func assertNotNil(t *testing.T, value interface{}, name string) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s is nil", name)
	}
}
