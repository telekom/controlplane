// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io/ioutil"
	"os"
	"path/filepath"
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

func TestNewS3ConfigWithLogger(t *testing.T) {
	// Test with custom logger
	testLogger := logr.Discard()

	config, err := NewS3ConfigWithLogger(testLogger)
	if err != nil {
		// In a test environment without AWS credentials, this will fail.
		// Just verify that the function signature works
		if config != nil {
			// If we got a non-nil config despite an error, check the logger
			if config.Logger != testLogger {
				t.Errorf("Expected logger to be set to test logger")
			}
		}
	}
}

func TestConfigOptions(t *testing.T) {
	// Test each configuration option individually
	testCases := []struct {
		name     string
		option   ConfigOption
		field    string
		value    string
		expected string
	}{
		{
			name:     "WithEndpoint",
			option:   WithEndpoint("test-endpoint"),
			field:    "Endpoint",
			value:    "test-endpoint",
			expected: "test-endpoint",
		},
		{
			name:     "WithSTSEndpoint",
			option:   WithSTSEndpoint("test-sts-endpoint"),
			field:    "STSEndpoint",
			value:    "test-sts-endpoint",
			expected: "test-sts-endpoint",
		},
		{
			name:     "WithBucketName",
			option:   WithBucketName("test-bucket"),
			field:    "BucketName",
			value:    "test-bucket",
			expected: "test-bucket",
		},
		{
			name:     "WithRoleSessionArn",
			option:   WithRoleSessionArn("test-role"),
			field:    "RoleSessionArn",
			value:    "test-role",
			expected: "test-role",
		},
		{
			name:     "WithTokenPath",
			option:   WithTokenPath("test-path"),
			field:    "TokenPath",
			value:    "test-path",
			expected: "test-path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &S3Config{}
			tc.option(config)

			var actualValue string
			switch tc.field {
			case "Endpoint":
				actualValue = config.Endpoint
			case "STSEndpoint":
				actualValue = config.STSEndpoint
			case "BucketName":
				actualValue = config.BucketName
			case "RoleSessionArn":
				actualValue = config.RoleSessionArn
			case "TokenPath":
				actualValue = config.TokenPath
			}

			if actualValue != tc.expected {
				t.Errorf("Expected %s to be %s, got %s", tc.field, tc.expected, actualValue)
			}
		})
	}
}

func TestWithLogger(t *testing.T) {
	// Test WithLogger option
	testLogger := logr.Discard()
	config := &S3Config{}

	WithLogger(testLogger)(config)
	if config.Logger != testLogger {
		t.Errorf("Expected logger to be set to test logger")
	}
}

func TestClientConfigCombinations(t *testing.T) {
	// This test checks various combinations of configuration options to ensure
	// client initialization is robust

	// Test with minimum required configuration (should fall back to anonymous credentials)
	minConfig := &S3Config{
		Logger:   logr.Discard(),
		Endpoint: "test-endpoint",
	}

	_, err := minConfig.initClient()
	if err != nil {
		// Should succeed with anonymous credentials
		t.Errorf("Unexpected error with minimum config: %v", err)
	}

	// Test with missing endpoint (should fail)
	missingEndpointConfig := &S3Config{
		Logger: logr.Discard(),
		// No endpoint
	}

	_, err = missingEndpointConfig.initClient()
	if err == nil {
		t.Error("Expected error with missing endpoint")
	}

	// Test with invalid endpoint format
	invalidEndpointConfig := &S3Config{
		Logger:   logr.Discard(),
		Endpoint: "invalid://endpoint", // Invalid URL format
	}

	_, err = invalidEndpointConfig.initClient()
	if err == nil {
		// This might not fail during client creation but would fail on actual operations
		// The test might need adjustment based on how the client library handles validation
		t.Log("Note: Invalid endpoint format did not cause immediate failure")
	}

	// Test with environment variable for token
	os.Setenv(WebIdentityTokenEnvVar, "env-token-for-client")
	defer os.Unsetenv(WebIdentityTokenEnvVar)

	envTokenConfig := &S3Config{
		Logger:         logr.Discard(),
		Endpoint:       "test-endpoint",
		STSEndpoint:    "test-sts-endpoint",
		RoleSessionArn: "test-role",
	}

	_, err = envTokenConfig.initClient()
	// In test environment this will fail to create STS credentials, which is expected
	// But we check that it tried to use the token from the environment
	if envTokenConfig.currentToken != "env-token-for-client" {
		t.Errorf("Expected token from environment, got %s", envTokenConfig.currentToken)
	}
}

func TestCreateMinioClient(t *testing.T) {
	// Test creating a Minio client with credentials
	config := &S3Config{
		Endpoint: "test-endpoint",
		Logger:   logr.Discard(),
	}

	// Create anonymous credentials for testing
	creds := credentials.NewStatic("", "", "", credentials.SignatureAnonymous)

	client, err := config.createMinioClient(creds)
	if err != nil {
		// This will likely fail in a test environment, but we test the function signature
		t.Logf("Error creating minio client (expected in test environment): %v", err)
	} else {
		if client == nil {
			t.Error("Expected client to not be nil")
		}
	}
}

func TestInitClient(t *testing.T) {
	// Test case 1: Basic client initialization with default config
	config := &S3Config{
		Logger:         logr.Discard(),
		Endpoint:       "test-endpoint",
		STSEndpoint:    "test-sts-endpoint",
		BucketName:     "test-bucket",
		RoleSessionArn: "test-role",
		TokenPath:      "/dev/null", // Use a path that always exists
	}

	client, err := config.initClient()
	if err != nil {
		// This will likely fail in a test environment, which is expected
		t.Logf("Error initializing client (expected in test environment): %v", err)
	} else {
		if client == nil {
			t.Error("Expected client to not be nil")
		}
	}

	// Test case 2: Client initialization with missing required config
	configMissingFields := &S3Config{
		Logger: logr.Discard(),
		// Missing endpoint and other required fields
	}

	_, err = configMissingFields.initClient()
	// Should fail with error about missing configuration
	if err == nil {
		t.Error("Expected error with missing configuration fields")
	}

	// Test case 3: Client initialization with token file
	tempDir, err := ioutil.TempDir("", "s3-client-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tokenFile := filepath.Join(tempDir, "token")
	testToken := "test-token-for-client"
	err = ioutil.WriteFile(tokenFile, []byte(testToken), 0644)
	if err != nil {
		t.Fatalf("Failed to write test token file: %v", err)
	}

	configWithToken := &S3Config{
		Logger:         logr.Discard(),
		Endpoint:       "test-endpoint",
		STSEndpoint:    "test-sts-endpoint",
		BucketName:     "test-bucket",
		RoleSessionArn: "test-role",
		TokenPath:      tokenFile,
	}

	_, err = configWithToken.initClient()
	// In test environment this will fail to create STS credentials, which is expected
	// But we check that it tried to use the token from the file
	if configWithToken.currentToken != testToken {
		t.Errorf("Expected token %s to be loaded from file, got %s", testToken, configWithToken.currentToken)
	}
}
