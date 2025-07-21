// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"fmt"
)

const (
	TypeErrNotFound        = "NotFound"
	TypeErrInvalidFileId   = "InvalidFileId"
	TypeErrFileExists      = "FileExists"
	TypeErrTooManyRequests = "TooManyRequests"
)

var _ error = &BackendError{}

// BackendError represents an error that occurred in the backend system
type BackendError struct {
	// FileId is the identifier of the file related to this error
	FileId string
	// Type categorizes the error
	Type string
	// Err is the underlying error
	Err error
}

// Error implements the error interface
func (e *BackendError) Error() string {
	return e.Type + ": " + e.Err.Error()
}

// NewBackendError creates a new BackendError with the given file ID, error, and type
func NewBackendError(fileId string, err error, typ string) *BackendError {
	return &BackendError{
		Type:   typ,
		FileId: fileId,
		Err:    err,
	}
}

// IsBackendError checks if the given error is a BackendError
func IsBackendError(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	return errors.As(err, &backendErr)
}

// ErrFileNotFound creates a BackendError for a file that was not found
func ErrFileNotFound(fileId string) *BackendError {
	if fileId == "" {
		return ErrNotFound()
	}
	err := fmt.Errorf("file with ID '%s' not found", fileId)
	return NewBackendError(fileId, err, TypeErrNotFound)
}

// ErrNotFound creates a generic "not found" BackendError
func ErrNotFound() *BackendError {
	return NewBackendError("", fmt.Errorf("resource not found"), TypeErrNotFound)
}

// IsNotFoundErr checks if the given error is a "not found" error
func IsNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Type == TypeErrNotFound
	}
	return false
}

// ErrInvalidFileId creates a BackendError for an invalid file ID
func ErrInvalidFileId(fileId string) *BackendError {
	err := fmt.Errorf("invalid file ID '%s'", fileId)
	return NewBackendError(fileId, err, TypeErrInvalidFileId)
}

// IsInvalidFileIdErr checks if the given error is an "invalid file ID" error
func IsInvalidFileIdErr(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Type == TypeErrInvalidFileId
	}
	return false
}

// ErrFileExists creates a BackendError for a file that already exists
func ErrFileExists(fileId string) *BackendError {
	err := fmt.Errorf("file with ID '%s' already exists", fileId)
	return NewBackendError(fileId, err, TypeErrFileExists)
}

// IsFileExistsErr checks if the given error is a "file exists" error
func IsFileExistsErr(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Type == TypeErrFileExists
	}
	return false
}

// ErrTooManyRequests creates a BackendError for rate limiting
func ErrTooManyRequests(fileId string) *BackendError {
	err := fmt.Errorf("too many requests")
	return NewBackendError(fileId, err, TypeErrTooManyRequests)
}

// IsTooManyRequestsErr checks if the given error is a "too many requests" error
func IsTooManyRequestsErr(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Type == TypeErrTooManyRequests
	}
	return false
}
