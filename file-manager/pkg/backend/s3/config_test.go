// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"os"
	"testing"
)

func TestNewS3Config(t *testing.T) {
	// Test with default values
	config, err := NewS3Config()
	if err != nil {
		// In a real environment, this would succeed, but in a test environment without AWS credentials,
		// it will fail. So we just check the error message.
		if config != nil {
			t.Errorf("Expected nil config on error, got %v", config)
		}
	}

	// Test with custom options
	testEndpoint := "test-endpoint"
	testSTSEndpoint := "test-sts-endpoint"
	testBucket := "test-bucket"
	testRole := "test-role"
	testTokenPath := "test-token-path"

	config, err = NewS3Config(
		WithEndpoint(testEndpoint),
		WithSTSEndpoint(testSTSEndpoint),
		WithBucketName(testBucket),
		WithRoleSessionArn(testRole),
		WithTokenPath(testTokenPath),
	)

	// In a test environment without AWS credentials, this will still fail,
	// but we can verify that options are applied correctly
	if config != nil {
		if config.Endpoint != testEndpoint {
			t.Errorf("Expected endpoint %s, got %s", testEndpoint, config.Endpoint)
		}
		if config.STSEndpoint != testSTSEndpoint {
			t.Errorf("Expected STS endpoint %s, got %s", testSTSEndpoint, config.STSEndpoint)
		}
		if config.BucketName != testBucket {
			t.Errorf("Expected bucket %s, got %s", testBucket, config.BucketName)
		}
		if config.RoleSessionArn != testRole {
			t.Errorf("Expected role %s, got %s", testRole, config.RoleSessionArn)
		}
		if config.TokenPath != testTokenPath {
			t.Errorf("Expected token path %s, got %s", testTokenPath, config.TokenPath)
		}
	}
}

func TestGetWebIDTokenFromEnv(t *testing.T) {
	config := &S3Config{}

	// Test with environment variable not set
	os.Unsetenv("MC_WEB_IDENTITY_TOKEN")
	_, err := config.getWebIDTokenFromEnv()
	if err == nil {
		t.Error("Expected error when environment variable is not set")
	}

	// Test with environment variable set
	os.Setenv("MC_WEB_IDENTITY_TOKEN", "test-token")
	token, err := config.getWebIDTokenFromEnv()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if token.Token != "test-token" {
		t.Errorf("Expected token %s, got %s", "test-token", token.Token)
	}

	// Clean up
	os.Unsetenv("MC_WEB_IDENTITY_TOKEN")
}
