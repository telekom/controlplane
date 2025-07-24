// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"github.com/go-logr/logr"
	"testing"
)

func TestUpdateBearerToken(t *testing.T) {
	// Create a config with initial settings
	config := &S3Config{
		Logger:       logr.Discard(),
		Endpoint:     "test-endpoint",
		STSEndpoint:  "test-sts-endpoint",
		BucketName:   "test-bucket",
		TokenPath:    "/dev/null", // Use a valid path that always exists
		currentToken: "",          // Start with empty token
	}

	// Test case 1: Update with a new token
	err := config.UpdateBearerToken("test-token-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if config.currentToken != "test-token-1" {
		t.Errorf("Expected token test-token-1, got %s", config.currentToken)
	}
	if config.currentCreds == nil {
		t.Error("Expected credentials to be created, got nil")
	}

	// Store the credentials for comparison
	firstCreds := config.currentCreds

	// Test case 2: Update with the same token - should not create new credentials
	err = config.UpdateBearerToken("test-token-1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if config.currentCreds != firstCreds {
		t.Error("Expected credentials to be the same, got different credentials")
	}

	// Test case 3: Update with a new token - should create new credentials
	err = config.UpdateBearerToken("test-token-2")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if config.currentToken != "test-token-2" {
		t.Errorf("Expected token test-token-2, got %s", config.currentToken)
	}
	if config.currentCreds == firstCreds {
		t.Error("Expected new credentials to be created, got the same credentials")
	}
}
