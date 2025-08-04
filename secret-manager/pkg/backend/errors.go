// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"errors"
	"fmt"
)

const (
	TypeErrForbidden       = "Forbidden"
	TypeErrNotFound        = "NotFound"
	TypeErrBadChecksum     = "BadChecksum"
	TypeErrInvalidSecretId = "InvalidSecretId"
	TypeErrTooManyRequests = "TooManyRequests"
)

var _ error = &BackendError{}

type BackendError struct {
	Id   SecretId
	Type string
	Err  error
}

func (e *BackendError) Error() string {
	return e.Type + ": " + e.Err.Error()
}

func NewBackendError(id SecretId, err error, typ string) *BackendError {
	return &BackendError{
		Type: typ,
		Id:   id,
		Err:  err,
	}
}

func IsBackendError(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	return errors.As(err, &backendErr)
}

func ErrSecretNotFound(id SecretId) *BackendError {
	if id == nil {
		return ErrNotFound()
	}
	err := fmt.Errorf("resource %s not found", id.String())
	return NewBackendError(id, err, TypeErrNotFound)
}

func ErrNotFound() *BackendError {
	return NewBackendError(nil, fmt.Errorf("resource not found"), TypeErrNotFound)
}

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

func ErrBadChecksum(id SecretId) *BackendError {
	err := fmt.Errorf("bad checksum for secret %s", id.String())
	bErr := NewBackendError(id, err, TypeErrBadChecksum)
	return bErr
}

func ErrInvalidSecretId(rawId string) *BackendError {
	err := fmt.Errorf("invalid secret id '%s'", rawId)
	return NewBackendError(nil, err, TypeErrInvalidSecretId)
}

func ErrIncorrectState(id SecretId, err error) *BackendError {
	return NewBackendError(id, err, "IncorrectState")
}

func IsIncorrectStateErr(err error) bool {
	if err == nil {
		return false
	}
	var backendErr *BackendError
	if errors.As(err, &backendErr) {
		return backendErr.Type == "IncorrectState"
	}
	return false
}

func Forbidden(id SecretId, err error) *BackendError {
	return NewBackendError(id, err, TypeErrForbidden)
}
