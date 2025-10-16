// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

var _ NotificationAdapter[ChatConfiguration] = MsTeamsAdapter{}

type MsTeamsAdapter struct {
	httpClient *http.Client
}

// NewMsTeamsAdapter creates a new instance of MsTeamsAdapter with default HTTP client
func NewMsTeamsAdapter() MsTeamsAdapter {
	return MsTeamsAdapter{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (e MsTeamsAdapter) Send(ctx context.Context, config ChatConfiguration, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Validate required parameters
	webhookURL := config.GetWebhookURL()
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	if body == "" {
		return fmt.Errorf("message body is required")
	}

	// Try to pretty print the JSON body for debugging
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(body), "", "  "); err == nil {
		// If it's valid JSON, base64 encode it for clean log output
		encodedBody := base64.StdEncoding.EncodeToString(prettyJSON.Bytes())
		log.V(1).Info("Sending to MS Teams",
			"webhook", webhookURL,
			"body (base64)", encodedBody,
		)
	} else {
		// If it's not valid JSON, base64 encode the original body
		encodedBody := base64.StdEncoding.EncodeToString([]byte(body))
		log.V(1).Info("Sending to MS Teams (non-JSON body)",
			"webhook", webhookURL,
			"body (base64)", encodedBody,
		)
	}

	// Create new request with context
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		webhookURL,
		bytes.NewBufferString(body),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := e.getHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}

	defer resp.Body.Close()

	// Read response body for error details
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for non-success status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	log.V(1).Info("Successfully sent message to MS Teams",
		"status_code", resp.StatusCode,
	)

	return nil
}

// getHTTPClient returns the HTTP client, initializing it if necessary
func (e MsTeamsAdapter) getHTTPClient() *http.Client {
	if e.httpClient == nil {
		e.httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	return e.httpClient
}
