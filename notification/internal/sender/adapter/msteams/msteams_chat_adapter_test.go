// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package msteams

import (
	"context"
	"encoding/json"
	adapter2 "github.com/telekom/controlplane/notification/internal/sender/adapter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockChatConfig is a test implementation of ChatConfiguration
type mockChatConfig struct {
	webhookURL string
}

func (m *mockChatConfig) IsNotificationConfig() {}

func (m *mockChatConfig) GetWebhookURL() string {
	return m.webhookURL
}

func TestNewMsTeamsAdapter(t *testing.T) {
	adapter := NewMsTeamsAdapter()

	assertNotNil(t, adapter, "NewMsTeamsAdapter()")
	assertNotNil(t, adapter.client, "adapter.client")
}

func TestNewMsTeamsAdapterWithConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *MsTeamsAdapterConfig
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name:   "empty config",
			config: &MsTeamsAdapterConfig{},
		},
		{
			name: "custom timeout",
			config: &MsTeamsAdapterConfig{
				HTTPClientConfig: HTTPClientConfig{
					Timeout: 60 * time.Second,
				},
			},
		},
		{
			name: "custom user agent",
			config: &MsTeamsAdapterConfig{
				HTTPClientConfig: HTTPClientConfig{
					UserAgent: "TestAgent/1.0",
				},
			},
		},
		{
			name: "custom retry settings",
			config: &MsTeamsAdapterConfig{
				HTTPClientConfig: HTTPClientConfig{
					MaxRetries:          5,
					RetryInitialBackoff: 500 * time.Millisecond,
					RetryMaxBackoff:     10 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewMsTeamsAdapterWithConfig(tt.config)

			assertNotNil(t, adapter, "NewMsTeamsAdapterWithConfig()")
			assertNotNil(t, adapter.client, "adapter.client")
		})
	}
}

func TestSend_Success(t *testing.T) {
	// Create mock server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if contentType := r.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Errorf("Expected application/json content type, got %s", contentType)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1"))
	}))
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"text": "Test message"}`

	err := adapter.Send(context.Background(), config, "", body)
	assertNoError(t, err, "Send()")
}

func TestSend_ValidationErrors(t *testing.T) {
	adapter := NewMsTeamsAdapter()

	tests := []struct {
		name      string
		config    adapter2.ChatChannelConfiguration
		body      string
		expectErr string
	}{
		{
			name:      "empty webhook URL",
			config:    &mockChatConfig{webhookURL: ""},
			body:      `{"text": "Test"}`,
			expectErr: "webhook URL is required",
		},
		{
			name:      "empty body",
			config:    &mockChatConfig{webhookURL: "https://example.com"},
			body:      "",
			expectErr: "message body is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.Send(context.Background(), tt.config, "", tt.body)

			assertError(t, err, "Send()")
			assertErrorContains(t, err, tt.expectErr)
		})
	}
}

func TestSend_HTTPErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		expectErr  string
	}{
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			response:   "Invalid request",
			expectErr:  "unexpected status code 400",
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			response:   "Not found",
			expectErr:  "unexpected status code 404",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			response:   "Server error",
			expectErr:  "unexpected status code 500",
		},
		{
			name:       "503 Service Unavailable",
			statusCode: http.StatusServiceUnavailable,
			response:   "Service unavailable",
			expectErr:  "unexpected status code 503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newSuccessServer(t, tt.statusCode, tt.response)
			defer server.Close()

			adapter := NewMsTeamsAdapter()
			config := &mockChatConfig{webhookURL: server.URL}
			body := `{"text": "Test"}`

			err := adapter.Send(context.Background(), config, "", body)

			assertError(t, err, "Send()")
			assertErrorContains(t, err, tt.expectErr)
		})
	}
}

func TestSend_MSTeamsStructuredError(t *testing.T) {
	// Create mock server that returns MS Teams structured error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		errorResponse := TeamsErrorResponse{}
		errorResponse.Error.Code = "BadRequest"
		errorResponse.Error.Message = "Invalid message card"
		errorResponse.Error.InnerError.Code = "MessageCardError"
		errorResponse.Error.InnerError.Message = "Invalid card format"
		errorResponse.Error.InnerError.Date = "2025-10-17T09:00:00Z"
		errorResponse.Error.InnerError.RequestID = "test-request-id-123"

		_ = json.NewEncoder(w).Encode(errorResponse)
	}))
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"invalid": "card"}`

	err := adapter.Send(context.Background(), config, "", body)

	assertError(t, err, "Send()")
	if err == nil {
		return // assertError should have failed the test
	}

	// Verify error contains structured error information
	errStr := err.Error()
	expectedParts := []string{
		"MS Teams API error",
		"status 400",
		"code=BadRequest",
		"message=Invalid message card",
		"inner_code=MessageCardError",
		"inner_message=Invalid card format",
		"request_id=test-request-id-123",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errStr, part) {
			t.Errorf("Expected error to contain %q, got %q", part, errStr)
		}
	}
}

func TestSend_ContextCancellation(t *testing.T) {
	// Create mock server with delay
	server := newDelayedServer(t, 100*time.Millisecond, http.StatusOK)
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"text": "Test"}`

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := adapter.Send(ctx, config, "", body)

	assertError(t, err, "Send() with cancelled context")
	assertErrorContains(t, err, "HTTP request failed")
}

func TestSend_ContextTimeout(t *testing.T) {
	// Create mock server with long delay
	server := newDelayedServer(t, 200*time.Millisecond, http.StatusOK)
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"text": "Test"}`

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := adapter.Send(ctx, config, "", body)

	assertError(t, err, "Send() with timeout")
}

func TestSend_RetryOnServerError(t *testing.T) {
	// Create mock server that fails twice, then succeeds
	server, attemptCount := newRetryServer(t, http.StatusInternalServerError, http.StatusOK, 2)
	defer server.Close()

	adapter := NewMsTeamsAdapterWithConfig(&MsTeamsAdapterConfig{
		HTTPClientConfig: HTTPClientConfig{
			MaxRetries:          3,
			RetryInitialBackoff: 10 * time.Millisecond,
			RetryMaxBackoff:     100 * time.Millisecond,
		},
	})

	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"text": "Test"}`

	err := adapter.Send(context.Background(), config, "", body)

	assertNoError(t, err, "Send() after retries")
	assertAttempts(t, *attemptCount, 3, "Retry on server error")
}

func TestParseError_StructuredError(t *testing.T) {
	errorResponse := TeamsErrorResponse{}
	errorResponse.Error.Code = "InvalidRequest"
	errorResponse.Error.Message = "The request is invalid"
	errorResponse.Error.InnerError.Code = "ValidationError"
	errorResponse.Error.InnerError.Message = "Field 'text' is required"
	errorResponse.Error.InnerError.RequestID = "req-456"

	body, err := json.Marshal(errorResponse)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	parsedErr := parseError(400, body)

	assertError(t, parsedErr, "parseError()")
	if parsedErr == nil {
		return // assertError should have failed the test
	}

	errStr := parsedErr.Error()

	expectedParts := []string{
		"MS Teams API error",
		"status 400",
		"code=InvalidRequest",
		"message=The request is invalid",
		"inner_code=ValidationError",
		"inner_message=Field 'text' is required",
		"request_id=req-456",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errStr, part) {
			t.Errorf("Expected error to contain %q, got %q", part, errStr)
		}
	}
}

func TestParseError_PlainTextError(t *testing.T) {
	plainTextError := "Bad Request: Invalid JSON"

	parsedErr := parseError(400, []byte(plainTextError))

	assertError(t, parsedErr, "parseError()")
	if parsedErr == nil {
		return // assertError should have failed the test
	}

	errStr := parsedErr.Error()

	if !strings.Contains(errStr, "unexpected status code 400") {
		t.Errorf("Expected error to contain status code, got %q", errStr)
	}

	if !strings.Contains(errStr, plainTextError) {
		t.Errorf("Expected error to contain plain text error, got %q", errStr)
	}
}

func TestParseError_EmptyBody(t *testing.T) {
	parsedErr := parseError(500, []byte{})

	assertError(t, parsedErr, "parseError()")
	if parsedErr == nil {
		return // assertError should have failed the test
	}

	errStr := parsedErr.Error()

	if !strings.Contains(errStr, "unexpected status code 500") {
		t.Errorf("Expected error to contain status code, got %q", errStr)
	}
}

func TestSend_RequestBodyVerification(t *testing.T) {
	expectedBody := `{"@type":"MessageCard","text":"Hello Teams"}`
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read request body
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1"))
	}))
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}

	err := adapter.Send(context.Background(), config, "", expectedBody)

	assertNoError(t, err, "Send()")

	if receivedBody != expectedBody {
		t.Errorf("Expected body %q, got %q", expectedBody, receivedBody)
	}
}

func TestSend_HeadersVerification(t *testing.T) {
	var headers http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("1"))
	}))
	defer server.Close()

	adapter := NewMsTeamsAdapter()
	config := &mockChatConfig{webhookURL: server.URL}
	body := `{"text": "Test"}`

	err := adapter.Send(context.Background(), config, "", body)

	assertNoError(t, err, "Send()")

	// Verify required headers
	contentType := headers.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type with application/json, got %s", contentType)
	}

	accept := headers.Get("Accept")
	if accept != "application/json" {
		t.Errorf("Expected Accept: application/json, got %s", accept)
	}

	userAgent := headers.Get("User-Agent")
	if userAgent == "" {
		t.Error("Expected User-Agent header to be set")
	}

	if !strings.Contains(userAgent, "TARDIS-Notification-Service") {
		t.Errorf("Expected User-Agent to contain TARDIS-Notification-Service, got %s", userAgent)
	}
}
