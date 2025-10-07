// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
)

func TestBucketFileDeleter_DeleteFile(t *testing.T) {
	// Test case 1: Nil config
	deleterNilConfig := NewBucketFileDeleter(nil)
	err := deleterNilConfig.DeleteFile(context.Background(), "valid/path/to/file")
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
	deleterNoClient := NewBucketFileDeleter(configNoClient)
	err = deleterNoClient.DeleteFile(context.Background(), "env/group/team/file.txt")
	if err == nil {
		t.Error("Expected error due to nil client, but got success")
	}
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

// Note: More comprehensive integration tests with actual MinIO client
// would require mocking the MinIO client or setting up a test MinIO instance.
// The current tests verify the basic error handling for invalid configurations.
//
// The following scenarios are covered indirectly through integration tests:
// - Successful file deletion
// - File not found (404) handling
// - Backend errors during deletion
// - Proper logging of delete operations
