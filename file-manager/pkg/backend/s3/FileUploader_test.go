// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
	"strings"
	"testing"
)

func TestS3FileUploader_UploadFile(t *testing.T) {
	// Create a mock config (we'll skip client initialization for unit testing)
	config := &S3Config{
		Endpoint:       "mock-endpoint",
		BucketName:     "mock-bucket",
		RoleSessionArn: "mock-role",
	}

	uploader := NewS3FileUploader(config)

	// Test case 1: Nil client validation through wrapper
	reader := strings.NewReader("test content")
	var r io.Reader = reader
	_, err := uploader.UploadFile(context.Background(), "env--group--team--file.txt", r, nil)
	if err == nil {
		t.Error("Expected error when client is nil")
	}

	// Restore config for next tests
	uploader.config = config

	// Test case 2: Nil reader is handled by the controller, not tested here anymore

	// Test case 3: Invalid fileId format
	// First make sure the config has a client to pass initial validation
	uploader.config.Client = &minio.Client{} // Mock client
	_, err = uploader.UploadFile(context.Background(), "invalid-file-id", r, nil)
	if err == nil {
		t.Error("Expected error when fileId format is invalid")
	}
	// Check if the error is an InvalidFileId error
	if err != nil && !backend.IsInvalidFileIdErr(err) {
		t.Errorf("Expected InvalidFileIdErr, got: %v", err)
	}

	// Note: A full test with mocked S3 client would be added in a future PR
	// That would test the complete flow with proper mocking of the S3 client
}

// TODO: Add proper mocked tests for the validation functionality in a separate PR
