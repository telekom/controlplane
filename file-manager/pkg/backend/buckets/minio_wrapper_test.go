// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/api/constants"
)

func TestMinioWrapper_ValidateClient(t *testing.T) {
	// Test case 1: Nil config
	nilConfigWrapper := NewMinioWrapper(nil)
	err := nilConfigWrapper.ValidateClient(context.Background())
	if err == nil {
		t.Error("Expected error when config is nil")
	}

	// Test case 2: Config without client
	configNoClient := &BucketConfig{
		Logger:     logr.Discard(),
		Endpoint:   "mock-endpoint",
		BucketName: "mock-bucket",
		Client:     nil,
	}
	wrapperNoClient := NewMinioWrapper(configNoClient)
	err = wrapperNoClient.ValidateClient(context.Background())
	if err == nil {
		t.Error("Expected error due to nil client, but got success")
	}

	// Test case 3: Valid config
	validConfig := &BucketConfig{
		Logger:     logr.Discard(),
		Endpoint:   "mock-endpoint",
		BucketName: "mock-bucket",
		Client:     &minio.Client{}, // This is not a fully functional client but sufficient for validation
	}
	wrapperValid := NewMinioWrapper(validConfig)
	err = wrapperValid.ValidateClient(context.Background())
	if err != nil {
		t.Errorf("Expected no error with valid client, got: %v", err)
	}
}

func TestMinioWrapper_ExtractMetadata(t *testing.T) {
	// Create wrapper with basic config
	config := &BucketConfig{
		Logger:   logr.Discard(),
		Endpoint: "mock-endpoint",
	}
	wrapper := NewMinioWrapper(config)

	objInfo := minio.ObjectInfo{
		ContentType: "text/plain",
		ETag:        "etag",
		Metadata: http.Header{
			"X-Amz-Meta-X-File-Checksum": []string{"abc123"},
		},
	}

	metadata := wrapper.ExtractMetadata(context.Background(), objInfo)

	// Verify metadata was extracted correctly
	if metadata[constants.XFileContentType] != "text/plain" {
		t.Errorf("Expected content type to be text/plain, got %s", metadata[constants.XFileContentType])
	}

	if metadata[constants.XFileChecksum] != "abc123" {
		t.Errorf("Expected checksum to be abc123, got %s", metadata[constants.XFileChecksum])
	}
}

func TestMinioWrapper_UpdateCredentialsFromContext(t *testing.T) {
	// Create a simple test
	config := &BucketConfig{
		Logger:   logr.Discard(),
		Endpoint: "mock-endpoint",
	}

	wrapper := NewMinioWrapper(config)

	// Just verify no panic with a basic context
	ctx := context.Background()
	// This should not panic
	wrapper.UpdateCredentialsFromContext(ctx)
}

func TestMinioWrapper_ValidateObjectMetadata(t *testing.T) {
	// Create a basic config for testing
	config := &BucketConfig{
		Logger:     logr.Discard(),
		Endpoint:   "mock-endpoint",
		BucketName: "mock-bucket",
	}

	wrapper := NewMinioWrapper(config)

	// Validate that client validation is called first
	err := wrapper.ValidateClient(context.Background())
	if err == nil {
		t.Error("Expected error with nil client, got success")
	}

	// Note: Testing actual validation logic would require mocking the bucket client responses
	// which is beyond the scope of this unit test. This should be covered in integration tests
	// or with a more sophisticated mocking setup.
}
