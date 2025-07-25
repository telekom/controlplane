// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
)

func TestS3FileDownloader_DownloadFile(t *testing.T) {
	// Test case 1: Nil config
	downloaderNilConfig := NewS3FileDownloader(nil)
	_, _, err := downloaderNilConfig.DownloadFile(context.Background(), "valid/path/to/file")
	if err == nil {
		t.Error("Expected error when config is nil")
	}

	// Test case 2: Config without client
	configNoClient := &S3Config{
		Logger:     logr.Discard(),
		Endpoint:   "mock-endpoint",
		BucketName: "mock-bucket",
		Client:     nil,
	}
	downloaderNoClient := NewS3FileDownloader(configNoClient)
	_, _, err = downloaderNoClient.DownloadFile(context.Background(), "env/group/team/file.txt")
	if err == nil {
		t.Error("Expected error due to nil client, but got success")
	}
}

// Moved the following tests to minio_wrapper_test.go:
// - TestS3FileDownloader_extractMetadata -> TestMinioWrapper_ExtractMetadata
// - TestS3FileDownloader_updateCredentialsFromContext -> TestMinioWrapper_UpdateCredentialsFromContext
// - Tests for getObjectInfo are now handled in TestMinioWrapper_GetObjectInfo

// downloadObject is still part of S3FileDownloader, but requires more complex mocking
// and is covered indirectly through other tests.
