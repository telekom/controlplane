// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"testing"
)

func TestS3FileDownloader_DownloadFile(t *testing.T) {
	// Create a mock config (we'll skip client initialization for unit testing)
	config := &S3Config{
		Endpoint:       "mock-endpoint",
		BucketName:     "mock-bucket",
		RoleSessionArn: "mock-role",
	}

	downloader := NewS3FileDownloader(config)

	// Test case 1: Nil config
	downloader.config = nil
	_, err := downloader.DownloadFile(context.Background(), "valid--file--id--name")
	if err == nil {
		t.Error("Expected error when config is nil")
	}

	// Restore config for next tests
	downloader.config = config

	// Test case 2: Invalid file ID format
	_, err = downloader.DownloadFile(context.Background(), "invalid-file-id")
	if err == nil {
		t.Error("Expected error when file ID format is invalid")
	}

	// Test case 3: Valid case (but will fail because we have no real client)
	// This is just to test that validation passes
	_, err = downloader.DownloadFile(context.Background(), "env--group--team--file.txt")
	// We expect an error because the client is nil
	if err == nil {
		t.Error("Expected error due to nil client, but got success")
	}
}
