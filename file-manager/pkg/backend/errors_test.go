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
