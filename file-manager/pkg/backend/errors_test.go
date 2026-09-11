// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackendError(t *testing.T) {
	err := errors.New("test error")
	fileId := "test-file-id"

	backendErr := NewBackendError(fileId, err, TypeErrNotFound)
	assert.Equal(t, fileId, backendErr.FileId)
	assert.Equal(t, TypeErrNotFound, backendErr.Type)
	assert.Equal(t, err, backendErr.Err)
	assert.Equal(t, "NotFound: test error", backendErr.Error())

	// Test error interface implementation
	var stdErr error = backendErr
	assert.NotNil(t, stdErr)
	assert.Equal(t, "NotFound: test error", stdErr.Error())
}

func TestIsBackendError(t *testing.T) {
	backendErr := ErrFileNotFound("test-file-id")
	regularErr := fmt.Errorf("regular error")

	assert.True(t, IsBackendError(backendErr))
	assert.False(t, IsBackendError(regularErr))
	assert.False(t, IsBackendError(nil))
}

func TestErrFileNotFound(t *testing.T) {
	fileId := "test-file-id"
	backendErr := ErrFileNotFound(fileId)

	assert.Equal(t, fileId, backendErr.FileId)
	assert.Equal(t, TypeErrNotFound, backendErr.Type)
	assert.Contains(t, backendErr.Error(), fileId)
	assert.True(t, IsNotFoundErr(backendErr))

	// Test with empty fileId
	backendErr = ErrFileNotFound("")
	assert.Equal(t, "", backendErr.FileId)
	assert.Equal(t, TypeErrNotFound, backendErr.Type)
	assert.True(t, IsNotFoundErr(backendErr))
}

func TestErrInvalidFileId(t *testing.T) {
	fileId := "invalid-id"
	backendErr := ErrInvalidFileId(fileId)

	assert.Equal(t, fileId, backendErr.FileId)
	assert.Equal(t, TypeErrInvalidFileId, backendErr.Type)
	assert.Contains(t, backendErr.Error(), fileId)
	assert.True(t, IsInvalidFileIdErr(backendErr))
}

func TestErrFileExists(t *testing.T) {
	fileId := "existing-file-id"
	backendErr := ErrFileExists(fileId)

	assert.Equal(t, fileId, backendErr.FileId)
	assert.Equal(t, TypeErrFileExists, backendErr.Type)
	assert.Contains(t, backendErr.Error(), fileId)
	assert.True(t, IsFileExistsErr(backendErr))
}

func TestErrTooManyRequests(t *testing.T) {
	fileId := "file-id"
	backendErr := ErrTooManyRequests(fileId)

	assert.Equal(t, fileId, backendErr.FileId)
	assert.Equal(t, TypeErrTooManyRequests, backendErr.Type)
	assert.True(t, IsTooManyRequestsErr(backendErr))
}

func TestBackendErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		errType  string
		expected int
	}{
		{"NotFound returns 404", TypeErrNotFound, 404},
		{"InvalidFileId returns 400", TypeErrInvalidFileId, 400},
		{"InvalidChecksum returns 400", TypeErrInvalidChecksum, 400},
		{"InvalidContentType returns 400", TypeErrInvalidContentType, 400},
		{"FileExists returns 409", TypeErrFileExists, 409},
		{"TooManyRequests returns 429", TypeErrTooManyRequests, 429},
		{"ClientInitialization returns 500", TypeErrClientInitialization, 500},
		{"UploadFailed returns 500", TypeErrUploadFailed, 500},
		{"DownloadFailed returns 500", TypeErrDownloadFailed, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewBackendError("file-id", fmt.Errorf("test"), tt.errType)
			assert.Equal(t, tt.expected, err.Code())
		})
	}
}

func TestBackendErrorCodeDefaultFallback(t *testing.T) {
	// Unknown type without StatusCode set should default to 500
	err := NewBackendError("file-id", fmt.Errorf("test"), "UnknownType")
	assert.Equal(t, 500, err.Code())
}

func TestBackendErrorCodeWithStatusCode(t *testing.T) {
	// Unknown type with explicit StatusCode should use that value
	err := NewBackendError("file-id", fmt.Errorf("test"), "CustomType").WithStatusCode(503)
	assert.Equal(t, 503, err.Code())
}

func TestWithStatusCode(t *testing.T) {
	err := NewBackendError("file-id", fmt.Errorf("test"), "SomeType")
	assert.Equal(t, 0, err.StatusCode)

	result := err.WithStatusCode(418)
	assert.Equal(t, 418, err.StatusCode)
	// WithStatusCode returns the same pointer for chaining
	assert.Equal(t, err, result)
}
