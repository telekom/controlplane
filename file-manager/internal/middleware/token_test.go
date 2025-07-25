// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"testing"
)

func TestExtractBearerTokenFromContext(t *testing.T) {
	// Test case 1: Token exists in context
	testToken := "test-token-123"
	ctx := WithBearerToken(context.Background(), testToken)

	token, err := ExtractBearerTokenFromContext(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if token != testToken {
		t.Errorf("Expected token %s, got %s", testToken, token)
	}

	// Test case 2: Token with Bearer prefix
	testToken = "Bearer test-token-456"
	ctx = WithBearerToken(context.Background(), testToken)

	token, err = ExtractBearerTokenFromContext(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if token != "test-token-456" {
		t.Errorf("Expected token test-token-456, got %s", token)
	}

	// Test case 3: No token in context
	emptyCtx := context.Background()
	_, err = ExtractBearerTokenFromContext(emptyCtx)
	if err == nil {
		t.Error("Expected error for missing token, got nil")
	}
}

func TestWithBearerToken(t *testing.T) {
	// Test adding token to context
	testToken := "test-token-789"
	ctx := context.Background()
	tokenCtx := WithBearerToken(ctx, testToken)

	// Verify token was added correctly
	tokenVal := tokenCtx.Value(BearerTokenKey)
	if tokenVal == nil {
		t.Error("Expected token in context, got nil")
	}

	token, ok := tokenVal.(string)
	if !ok {
		t.Error("Expected token value to be string")
	}

	if token != testToken {
		t.Errorf("Expected token %s, got %s", testToken, token)
	}
}
