// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

func TestBucketFileUploader_UploadFile(t *testing.T) {
	// Create a mock config (we'll skip client initialization for unit testing)
	config := &BucketConfig{
		Endpoint:   "mock-endpoint",
		BucketName: "mock-bucket",
	}

	uploader := NewBucketFileUploader(config)

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

	// Note: A full test with mocked client would be added in a future PR
	// That would test the complete flow with proper mocking of the client
}

// TODO: Add proper mocked tests for the validation functionality in a separate PR
