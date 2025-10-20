// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package adapter provides notification adapters for various communication channels.
//
// MsTeamsAdapter sends notifications to Microsoft Teams via webhooks.
// The HTTP client configuration is handled separately in http_client.go.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/go-resty/resty/v2"
)

const (
	// Default User-Agent for MS Teams requests
	defaultUserAgent = "TARDIS-Notification-Service/1.0"
)

var _ NotificationAdapter[ChatChannelConfiguration] = &MsTeamsAdapter{}

// MsTeamsAdapter is an adapter for sending notifications to Microsoft Teams via webhooks
type MsTeamsAdapter struct {
	client *resty.Client
}

// MsTeamsAdapterConfig holds configuration for the MsTeamsAdapter.
// It wraps HTTPClientConfig and adds MS Teams specific settings.
type MsTeamsAdapterConfig struct {
	HTTPClientConfig
}

// NewMsTeamsAdapter creates a new instance of MsTeamsAdapter with default configuration
func NewMsTeamsAdapter() *MsTeamsAdapter {
	return NewMsTeamsAdapterWithConfig(nil)
}

// NewMsTeamsAdapterWithConfig creates a new instance of MsTeamsAdapter with custom configuration
func NewMsTeamsAdapterWithConfig(config *MsTeamsAdapterConfig) *MsTeamsAdapter {
	// Apply defaults if config is nil
	if config == nil {
		config = &MsTeamsAdapterConfig{}
	}

	// Set default User-Agent if not provided
	if config.UserAgent == "" {
		config.UserAgent = defaultUserAgent
	}

	// Set default retry condition if not provided
	if config.RetryConditionFunc == nil {
		config.RetryConditionFunc = DefaultRetryCondition
	}

	// Create HTTP client using the shared factory
	client := NewRestyClient(&config.HTTPClientConfig)

	// Set MS Teams specific headers
	client.SetHeader("Content-Type", "application/json; charset=utf-8").
		SetHeader("Accept", "application/json")

	return &MsTeamsAdapter{
		client: client,
	}
}

// TeamsErrorResponse represents the error response from MS Teams webhook
type TeamsErrorResponse struct {
	Error struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		InnerError struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Date      string `json:"date"`
			RequestID string `json:"request-id"`
		} `json:"innerError"`
	} `json:"error"`
}

// Send sends a notification to Microsoft Teams with retry logic and proper error handling.
// The title parameter is ignored as MS Teams determines the title from the card content.
func (e MsTeamsAdapter) Send(ctx context.Context, config ChatChannelConfiguration, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Validate required parameters
	webhookURL := config.GetWebhookURL()
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	if body == "" {
		return fmt.Errorf("message body is required")
	}

	// Log request if verbose logging is enabled
	if log.V(1).Enabled() {
		log.V(1).Info("Sending request to MS Teams",
			"webhook", webhookURL,
			"body_size", len(body),
		)
	}

	// Send request with automatic retry via go-resty
	resp, err := e.client.R().
		SetContext(ctx).
		SetBody(body).
		Post(webhookURL)

	if err != nil {
		log.Error(err, "HTTP request failed",
			"webhook", webhookURL,
		)
		return fmt.Errorf("HTTP request failed: %w", err)
	}

	// Check for non-success status codes
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return parseError(resp.StatusCode(), resp.Body())
	}

	log.V(1).Info("Successfully sent message to MS Teams",
		"status_code", resp.StatusCode(),
		"response_size", len(resp.Body()),
		"duration", resp.Time(),
	)

	return nil
}

// parseError attempts to parse structured error response from MS Teams
func parseError(statusCode int, respBody []byte) error {
	// Try to parse as structured error
	var teamsErr TeamsErrorResponse
	if err := json.Unmarshal(respBody, &teamsErr); err == nil && teamsErr.Error.Code != "" {
		return fmt.Errorf("MS Teams API error (status %d): code=%s, message=%s, inner_code=%s, inner_message=%s, request_id=%s",
			statusCode,
			teamsErr.Error.Code,
			teamsErr.Error.Message,
			teamsErr.Error.InnerError.Code,
			teamsErr.Error.InnerError.Message,
			teamsErr.Error.InnerError.RequestID,
		)
	}

	// Fallback to raw response
	return fmt.Errorf("unexpected status code %d: %s", statusCode, string(respBody))
}
