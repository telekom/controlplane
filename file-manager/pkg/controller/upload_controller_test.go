// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/api/constants"
)

// MockFileUploader is a mock implementation of the FileUploader interface for testing
type MockFileUploader struct {
	UploadIdCalled string
	UploadReader   io.Reader
	UploadMetadata map[string]string
	MockReturnId   string
	MockError      error
}

// UploadFile is a mock implementation that records the inputs passed to it
func (m *MockFileUploader) UploadFile(ctx context.Context, fileId string, reader io.Reader, metadata map[string]string) (string, error) {
	// Record the inputs that were passed to verify they were passed correctly
	m.UploadIdCalled = fileId
	m.UploadReader = reader
	m.UploadMetadata = metadata

	if m.MockError != nil {
		return "", m.MockError
	}

	return m.MockReturnId, nil
}

func TestUploadController_UploadFile(t *testing.T) {
	// Create mock uploader
	mockUploader := &MockFileUploader{
		MockReturnId: "test-file-id",
		MockError:    nil,
	}

	// Create controller with mock uploader
	controller := NewUploadController(mockUploader)

	// Test case 1: Valid fileId
	fileId := "env--group--team--file.txt"
	reader := strings.NewReader("test content")
	var r io.Reader = reader
	metadata := map[string]string{constants.XFileChecksum: "test-checksum"}

	resultId, err := controller.UploadFile(context.Background(), fileId, &r, metadata)

	// Check that there was no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the correct fileId was passed to the uploader
	if mockUploader.UploadIdCalled != fileId {
		t.Errorf("Expected fileId %s to be passed to uploader, got %s",
			fileId, mockUploader.UploadIdCalled)
	}

	// Check that the result ID is what we expected
	if resultId != mockUploader.MockReturnId {
		t.Errorf("Expected result ID %s, got %s", mockUploader.MockReturnId, resultId)
	}

	// Reader has been passed through, but we can't compare interfaces directly
	// Just ensure it's not nil
	if mockUploader.UploadReader == nil {
		t.Error("Reader was not correctly passed to uploader")
	}

	// Check that content type was added to metadata
	contentType := mockUploader.UploadMetadata["X-File-Content-Type"]
	if contentType == "" {
		t.Error("Content type was not added to metadata")
	}
	expectedType := "text/plain; charset=utf-8" // Go's mime package adds charset for text types
	if contentType != expectedType {
		t.Errorf("Expected content type %s for .txt file, got %s", expectedType, contentType)
	}

	// Test case 2: Invalid fileId should fail with error
	invalidFileId := "invalid-file-id"
	_, err = controller.UploadFile(context.Background(), invalidFileId, &r, metadata)

	// Check that there was an error for invalid fileId
	if err == nil {
		t.Error("Expected error for invalid fileId, got nil")
	}

	// Test case 3: Nil reader should fail with error
	_, err = controller.UploadFile(context.Background(), fileId, nil, metadata)

	// Check that there was an error for nil reader
	if err == nil {
		t.Error("Expected error for nil reader, got nil")
	}

	// Test case 4: Uploader returning error should be propagated
	mockUploader.MockError = errors.New("upload failed")
	_, err = controller.UploadFile(context.Background(), fileId, &r, metadata)

	// Check that the error was propagated
	if err == nil {
		t.Error("Expected error to be propagated from uploader, got nil")
	}
}
