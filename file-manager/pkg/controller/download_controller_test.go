// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"testing"
)

// MockFileDownloader is a mock implementation of the FileDownloader interface for testing
type MockFileDownloader struct {
	DownloadPathCalled string
	MockContent        string
	MockMetadata       map[string]string
	MockError          error
}

// DownloadFile is a mock implementation that records the path that was passed to it
func (m *MockFileDownloader) DownloadFile(ctx context.Context, path string) (*io.Writer, map[string]string, error) {
	// Record the path that was passed to verify conversion happened correctly
	m.DownloadPathCalled = path

	if m.MockError != nil {
		return nil, nil, m.MockError
	}

	// Create a writer with mock content
	var writer io.Writer = &mockWriter{content: m.MockContent}
	return &writer, m.MockMetadata, nil
}

// mockWriter implements io.Writer for testing purposes
type mockWriter struct {
	content string
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestDownloadController_DownloadFile(t *testing.T) {
	// Create mock downloader
	mockDownloader := &MockFileDownloader{
		MockContent:  "test file content",
		MockMetadata: map[string]string{"X-File-Content-Type": "text/plain"},
		MockError:    nil,
	}

	// Create controller with mock downloader
	controller := NewDownloadController(mockDownloader)

	// Test case 1: Valid fileId with proper conversion
	fileId := "env--group--team--file.txt"
	expectedPath := "env/group/team/file.txt"

	_, _, err := controller.DownloadFile(context.Background(), fileId)

	// Check that there was no error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the correct path was passed to the downloader
	if mockDownloader.DownloadPathCalled != expectedPath {
		t.Errorf("Expected path %s to be passed to downloader, got %s",
			expectedPath, mockDownloader.DownloadPathCalled)
	}

	// Test case 2: Invalid fileId should fail with error
	invalidFileId := "invalid-file-id"
	_, _, err = controller.DownloadFile(context.Background(), invalidFileId)

	// Check that there was an error for invalid fileId
	if err == nil {
		t.Error("Expected error for invalid fileId, got nil")
	}
}
