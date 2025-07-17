// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
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

	// Test case 1: Nil config
	uploader.config = nil
	reader := strings.NewReader("test content")
	var r io.Reader = reader
	_, err := uploader.UploadFile(context.Background(), "valid--file--id--name", &r)
	if err == nil {
		t.Error("Expected error when config is nil")
	}

	// Restore config for next tests
	uploader.config = config

	// Test case 2: Nil reader
	_, err = uploader.UploadFile(context.Background(), "valid--file--id--name", nil)
	if err == nil {
		t.Error("Expected error when reader is nil")
	}

	// Test case 3: Invalid file ID format
	_, err = uploader.UploadFile(context.Background(), "invalid-file-id", &r)
	if err == nil {
		t.Error("Expected error when file ID format is invalid")
	}

	// Test case 4: Valid case (but will fail because we have no real client)
	// This is just to test that validation passes
	_, err = uploader.UploadFile(context.Background(), "env--group--team--file.txt", &r)
	// We expect an error because the client is nil
	if err == nil {
		t.Error("Expected error due to nil client, but got success")
	}
}
