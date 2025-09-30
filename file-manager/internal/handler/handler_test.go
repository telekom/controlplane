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
	UploadMock   func(ctx context.Context, fileId string, file io.Reader, metadata map[string]string) (string, error)
	DownloadMock func(ctx context.Context, fileId string) (io.Reader, map[string]string, error)
	DeleteMock   func(ctx context.Context, fileId string) error
}

func (m *MockController) UploadFile(ctx context.Context, fileId string, file io.Reader, metadata map[string]string) (string, error) {
	return m.UploadMock(ctx, fileId, file, metadata)
}

func (m *MockController) DownloadFile(ctx context.Context, fileId string) (io.Reader, map[string]string, error) {
	return m.DownloadMock(ctx, fileId)
}

func (m *MockController) DeleteFile(ctx context.Context, fileId string) error {
	return m.DeleteMock(ctx, fileId)
}

func TestHandler_UploadFile(t *testing.T) {
	// Create mock controller
	mockCtrl := &MockController{
		UploadMock: func(ctx context.Context, fileId string, file io.Reader, metadata map[string]string) (string, error) {
			if fileId == "success--test--case--file.txt" {
				return fileId, nil
			}
			return "", errors.New("mock error")
		},
	}

	h := NewHandler(mockCtrl)

	// Test case 1: Successful upload
	reader := strings.NewReader("test content")

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
	reader = strings.NewReader("test content")
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
		DownloadMock: func(ctx context.Context, fileId string) (io.Reader, map[string]string, error) {
			if fileId == "success--test--case--file.txt" {
				buf := bytes.NewBuffer([]byte("test content"))
				metadata := map[string]string{
					"X-File-Content-Type": "text/plain",
					"X-File-Checksum":     "abc123",
				}
				return buf, metadata, nil
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

	downloadResponse := response.(api.DownloadFile200ApplicationoctetStreamResponse)
	rawContent, _ := io.ReadAll(downloadResponse.Body)
	if string(rawContent) != "test content" {
		t.Error("Expected response of download to have body \"test content\"")
	}

	// Test case 2: Download error
	request.FileId = "error--case"
	_, err = h.DownloadFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error from controller")
	}
}

func TestHandler_DeleteFile(t *testing.T) {
	// Create mock controller
	mockCtrl := &MockController{
		DeleteMock: func(ctx context.Context, fileId string) error {
			if fileId == "success--test--case--file.txt" {
				return nil
			}
			if fileId == "not--found--file.txt" {
				return errors.New("FileNotFound: file not found")
			}
			return errors.New("mock error")
		},
	}

	h := NewHandler(mockCtrl)

	// Test case 1: Successful deletion
	request := api.DeleteFileRequestObject{
		FileId: "success--test--case--file.txt",
	}

	response, err := h.DeleteFile(context.Background(), request)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if _, ok := response.(api.DeleteFile204Response); !ok {
		t.Error("Expected DeleteFile204Response")
	}

	// Test case 2: File not found
	request.FileId = "not--found--file.txt"
	_, err = h.DeleteFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error for file not found")
	}

	// Test case 3: Delete error
	request.FileId = "error--case"
	_, err = h.DeleteFile(context.Background(), request)
	if err == nil {
		t.Error("Expected error from controller")
	}
}
