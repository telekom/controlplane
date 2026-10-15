// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorTypeCheckers(t *testing.T) {
	const fileId = "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60"

	tests := []struct {
		name     string
		err      *BackendError
		is       func(error) bool
		wantCode int
	}{
		{"not found", ErrFileNotFound(fileId), IsNotFoundErr, 404},
		{"invalid file id", ErrInvalidFileId(fileId), IsInvalidFileIdErr, 400},
		{"file exists", ErrFileExists(fileId), IsFileExistsErr, 409},
		{"too many requests", ErrTooManyRequests(fileId), IsTooManyRequestsErr, 429},
		{"invalid checksum", ErrInvalidChecksum(fileId, "a", "b"), IsInvalidChecksumErr, 400},
		{"invalid content type", ErrInvalidContentType(fileId, "a", "b"), IsInvalidContentTypeErr, 400},
		{"client initialization", ErrClientInitialization("boom"), IsClientInitializationErr, 500},
		{"upload failed", ErrUploadFailed(fileId, "boom"), IsUploadFailedErr, 500},
		{"download failed", ErrDownloadFailed(fileId, "boom"), IsDownloadFailedErr, 500},
	}

	other := NewBackendError(fileId, errors.New("other"), "Other")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.is(tt.err))
			assert.True(t, tt.is(fmt.Errorf("wrapped: %w", tt.err)))
			assert.False(t, tt.is(nil))
			assert.False(t, tt.is(errors.New("plain")))
			assert.False(t, tt.is(other))
			assert.Equal(t, tt.wantCode, tt.err.Code())
		})
	}
}

func TestBackendErrorCodeDefault(t *testing.T) {
	err := NewBackendError("", errors.New("boom"), "Custom")
	assert.Equal(t, 500, err.Code())
	assert.Equal(t, 418, err.WithStatusCode(418).Code())
}
