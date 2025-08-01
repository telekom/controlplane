// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/internal/api"
)

// MockController is a mock implementation of the controller.Controller interface for testing
type MockController struct {
	UploadMock   func(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error)
	DownloadMock func(ctx context.Context, fileId string) (*io.Writer, map[string]string, error)
}

func (m *MockController) UploadFile(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error) {
	return m.UploadMock(ctx, fileId, file, metadata)
}

func (m *MockController) DownloadFile(ctx context.Context, fileId string) (*io.Writer, map[string]string, error) {
	return m.DownloadMock(ctx, fileId)
}

func TestHandler_UploadFile(t *testing.T) {
	// Create mock controller
	mockCtrl := &MockController{
		UploadMock: func(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error) {
			if fileId == "success--test--case--file.txt" {
				return fileId, nil
			}
			return "", errors.New("mock error")
		},
	}

	h := NewHandler(mockCtrl)

	// Test case 1: Successful upload
	fileContent := strings.NewReader("test content")
	var reader io.Reader = fileContent

	contentType := "text/plain"
	checksum := "abc123"
	request := api.UploadFileRequestObject{
		FileId: "success--test--case--file.txt",
		Body:   reader,
		Params: api.UploadFileParams{
			XFileContentType: &contentType,
			XFileChecksum:    &checksum,
		},
	}

	response, err := h.UploadFile(context.Background(), request)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if resp, ok := response.(api.UploadFile200JSONResponse); ok {
		if resp.Body.Id != "success--test--case--file.txt" {
			t.Errorf("Expected file ID %s, got %s", "success--test--case--file.txt", resp.Body.Id)
		}
	} else {
		t.Error("Expected UploadFile200JSONResponse")
	}

	// Test case 2: Nil body
	request.Body = nil
	_, err = h.UploadFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error for nil body")
	}

	// Test case 3: Upload error
	fileContent = strings.NewReader("test content")
	reader = fileContent
	request.Body = reader
	request.FileId = "error--case"
	_, err = h.UploadFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error from controller")
	}
}

func TestHandler_DownloadFile(t *testing.T) {
	// Create mock controller
	mockCtrl := &MockController{
		DownloadMock: func(ctx context.Context, fileId string) (*io.Writer, map[string]string, error) {
			if fileId == "success--test--case--file.txt" {
				buf := bytes.NewBuffer([]byte("test content"))
				var w io.Writer = buf
				metadata := map[string]string{
					"X-File-Content-Type": "text/plain",
					"X-File-Checksum":     "abc123",
				}
				return &w, metadata, nil
			}
			return nil, nil, errors.New("mock error")
		},
	}

	h := NewHandler(mockCtrl)

	// Test case 1: Successful download
	request := api.DownloadFileRequestObject{
		FileId: "success--test--case--file.txt",
	}

	response, err := h.DownloadFile(context.Background(), request)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if _, ok := response.(api.DownloadFile200ApplicationoctetStreamResponse); !ok {
		t.Error("Expected DownloadFile200BinaryResponse")
	}

	// Test case 2: Download error
	request.FileId = "error--case"
	_, err = h.DownloadFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error from controller")
	}
}
