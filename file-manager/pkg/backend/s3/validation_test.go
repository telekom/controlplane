// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
	"testing"
)

// MockMinioWrapper provides a mock implementation for testing the validator
type MockMinioWrapper struct {
	ClientValid      bool
	MockObjectInfo   minio.ObjectInfo
	MockGetInfoError error
}

func (m *MockMinioWrapper) ValidateClient(ctx context.Context) error {
	if !m.ClientValid {
		return errors.New("mock client validation failed")
	}
	return nil
}

func (m *MockMinioWrapper) GetObjectInfo(ctx context.Context, path string) (minio.ObjectInfo, error) {
	if m.MockGetInfoError != nil {
		return minio.ObjectInfo{}, m.MockGetInfoError
	}
	return m.MockObjectInfo, nil
}

func TestObjectMetadataValidator_ValidateObjectMetadata(t *testing.T) {
	// Test case 1: Client validation failure
	mockWrapper := &MockMinioWrapper{
		ClientValid: false,
	}
	validator := NewObjectMetadataValidator(mockWrapper)

	err := validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "abc123")
	if err == nil {
		t.Error("Expected error when client validation fails, got nil")
	}

	// Test case 2: GetObjectInfo failure
	mockWrapper = &MockMinioWrapper{
		ClientValid:      true,
		MockGetInfoError: errors.New("mock GetObjectInfo error"),
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "abc123")
	if err == nil {
		t.Error("Expected error when GetObjectInfo fails, got nil")
	}

	// Test case 3: Content type mismatch
	mockWrapper = &MockMinioWrapper{
		ClientValid: true,
		MockObjectInfo: minio.ObjectInfo{
			ContentType: "application/json",
			ETag:        "abc123",
		},
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "abc123")
	if err == nil {
		t.Error("Expected error when content type mismatches, got nil")
	}

	// Test case 4: Checksum mismatch
	mockWrapper = &MockMinioWrapper{
		ClientValid: true,
		MockObjectInfo: minio.ObjectInfo{
			ContentType: "text/plain",
			ETag:        "different-checksum",
		},
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "abc123")
	if err == nil {
		t.Error("Expected error when checksum mismatches, got nil")
	}

	// Test case 5: Successful validation
	mockWrapper = &MockMinioWrapper{
		ClientValid: true,
		MockObjectInfo: minio.ObjectInfo{
			ContentType: "text/plain",
			ETag:        "abc123",
		},
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "abc123")
	if err != nil {
		t.Errorf("Expected successful validation, got error: %v", err)
	}

	// Test case 6: Empty expectations (no validation)
	mockWrapper = &MockMinioWrapper{
		ClientValid: true,
		MockObjectInfo: minio.ObjectInfo{
			ContentType: "anything",
			ETag:        "anything",
		},
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "", "")
	if err != nil {
		t.Errorf("Expected successful validation with empty expectations, got error: %v", err)
	}

	// Test case 7: User metadata checksum
	mockWrapper = &MockMinioWrapper{
		ClientValid: true,
		MockObjectInfo: minio.ObjectInfo{
			ContentType: "text/plain",
			ETag:        "",
			UserMetadata: map[string]string{
				identifier.XFileChecksum: "user-checksum",
			},
		},
	}
	validator = NewObjectMetadataValidator(mockWrapper)

	err = validator.ValidateObjectMetadata(context.Background(), "test/path", "text/plain", "user-checksum")
	if err != nil {
		t.Errorf("Expected successful validation with user metadata checksum, got error: %v", err)
	}
}
