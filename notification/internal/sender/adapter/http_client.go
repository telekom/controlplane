// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"net"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	// Default HTTP client configuration values
	defaultTimeout             = 30 * time.Second
	defaultMaxIdleConns        = 100
	defaultMaxConnsPerHost     = 10
	defaultIdleConnTimeout     = 90 * time.Second
	defaultTLSHandshakeTimeout = 10 * time.Second
	defaultMaxRetries          = 3
	defaultRetryInitialBackoff = 1 * time.Second
	defaultRetryMaxBackoff     = 30 * time.Second
	defaultDialTimeout         = 10 * time.Second
	defaultKeepAlive           = 30 * time.Second
)

// HTTPClientConfig holds configuration for creating HTTP clients with go-resty
type HTTPClientConfig struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	MaxRetries          int
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	UserAgent           string
	Debug               bool
	// RetryConditionFunc allows custom retry logic
	RetryConditionFunc func(*resty.Response, error) bool
	// Optional hooks for custom behavior
	OnBeforeRequest func(*resty.Client, *resty.Request) error
	OnAfterResponse func(*resty.Client, *resty.Response) error
	OnError         func(*resty.Request, error)
}

// NewRestyClient creates a production-ready resty client with the given configuration.
// This function encapsulates all the technical HTTP client setup including:
// - Connection pooling and timeouts
// - Retry logic with exponential backoff
// - HTTP/2 support
// - Custom transport configuration
func NewRestyClient(config *HTTPClientConfig) *resty.Client {
	// Apply defaults if config is nil
	if config == nil {
		config = &HTTPClientConfig{}
	}

	// Set defaults for zero values
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = defaultMaxIdleConns
	}
	if config.MaxConnsPerHost == 0 {
		config.MaxConnsPerHost = defaultMaxConnsPerHost
	}
	if config.IdleConnTimeout == 0 {
		config.IdleConnTimeout = defaultIdleConnTimeout
	}
	if config.TLSHandshakeTimeout == 0 {
		config.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = defaultMaxRetries
	}
	if config.RetryInitialBackoff == 0 {
		config.RetryInitialBackoff = defaultRetryInitialBackoff
	}
	if config.RetryMaxBackoff == 0 {
		config.RetryMaxBackoff = defaultRetryMaxBackoff
	}

	// Configure custom transport with connection pooling and timeouts
	// Start with http.DefaultTransport settings to preserve proxy and TLS config
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Override with custom settings
	transport.MaxIdleConns = config.MaxIdleConns
	transport.MaxIdleConnsPerHost = config.MaxConnsPerHost
	transport.MaxConnsPerHost = config.MaxConnsPerHost
	transport.IdleConnTimeout = config.IdleConnTimeout
	transport.TLSHandshakeTimeout = config.TLSHandshakeTimeout
	transport.DialContext = (&net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}).DialContext
	transport.ForceAttemptHTTP2 = true

	// Create resty client with configuration
	client := resty.New().
		SetTimeout(config.Timeout).
		SetRetryCount(config.MaxRetries).
		SetRetryWaitTime(config.RetryInitialBackoff).
		SetRetryMaxWaitTime(config.RetryMaxBackoff).
		SetTransport(transport).
		SetDebug(config.Debug)

	// Set User-Agent if provided
	if config.UserAgent != "" {
		client.SetHeader("User-Agent", config.UserAgent)
	}

	// Add custom retry condition if provided
	if config.RetryConditionFunc != nil {
		client.AddRetryCondition(config.RetryConditionFunc)
	}

	// Register optional hooks
	if config.OnBeforeRequest != nil {
		client.OnBeforeRequest(config.OnBeforeRequest)
	}
	if config.OnAfterResponse != nil {
		client.OnAfterResponse(config.OnAfterResponse)
	}
	if config.OnError != nil {
		client.OnError(config.OnError)
	}

	return client
}

// DefaultRetryCondition returns a standard retry condition function that retries on:
// - Network errors
// - 5xx server errors
// - 429 Too Many Requests
// - 408 Request Timeout
func DefaultRetryCondition(r *resty.Response, err error) bool {
	// Retry on network errors
	if err != nil {
		return true
	}

	// No error and no response - don't retry
	if r == nil {
		return false
	}

	// Retry on specific HTTP status codes
	statusCode := r.StatusCode()
	return statusCode >= 500 || // 5xx server errors
		statusCode == 429 || // Too Many Requests
		statusCode == 408 // Request Timeout
}
