// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"slices"
	"time"
)

type ApiResponse interface {
	StatusCode() int
}

// ApiError represents an error from the Keycloak API.
// It implements the ctrlerrors.BlockedError, ctrlerrors.RetryableError, and
// ctrlerrors.RetryableWithDelayError interfaces via duck-typing, so that
// errors propagated up the call stack are correctly classified by
// ctrlerrors.HandleError without introducing a direct import dependency.
type ApiError interface {
	error
	Retriable() bool
	IsRetryable() bool
	IsBlocked() bool
	RetryDelay() time.Duration
}

type apiError struct {
	statusCode   int
	message      string
	retryAllowed bool
	retryDelay   time.Duration
}

func (e *apiError) Error() string {
	return e.message
}

// Retriable is the original Keycloak-specific method, kept for backward compatibility.
func (e *apiError) Retriable() bool {
	return e.retryAllowed
}

// IsRetryable satisfies the ctrlerrors.RetryableError interface.
func (e *apiError) IsRetryable() bool {
	return e.retryAllowed
}

// IsBlocked satisfies the ctrlerrors.BlockedError interface.
// A 4xx error (non-retryable) is considered blocked — the request is invalid
// and retrying with the same parameters will not succeed.
func (e *apiError) IsBlocked() bool {
	return !e.retryAllowed
}

// RetryDelay satisfies the ctrlerrors.RetryableWithDelayError interface.
func (e *apiError) RetryDelay() time.Duration {
	return e.retryDelay
}

func CheckStatusCode(res ApiResponse, okStatusCodes ...int) ApiError {
	return CheckHTTPStatus(res.StatusCode(), okStatusCodes...)
}

// CheckHTTPStatus classifies a raw HTTP status code into an ApiError.
// It returns nil when statusCode is in okStatusCodes.
// This is the lower-level sibling of CheckStatusCode for callers that do not
// have an ApiResponse (e.g. oauth2.RetrieveError, inline status checks).
func CheckHTTPStatus(statusCode int, okStatusCodes ...int) ApiError {
	if slices.Contains(okStatusCodes, statusCode) {
		return nil
	}

	if statusCode == http.StatusTooManyRequests {
		return &apiError{
			statusCode:   statusCode,
			message:      fmt.Sprintf("Keycloak rate limit error (%d)", statusCode),
			retryAllowed: true,
			retryDelay:   3 * time.Second,
		}
	}

	if statusCode >= http.StatusInternalServerError {
		return &apiError{
			statusCode:   statusCode,
			message:      fmt.Sprintf("Keycloak server error (%d)", statusCode),
			retryAllowed: true,
		}
	}

	return &apiError{
		statusCode:   statusCode,
		message:      fmt.Sprintf("Keycloak client error (%d)", statusCode),
		retryAllowed: false,
	}
}

// IsNotFound returns true if the error is an ApiError with a 404 status code.
// This is useful for distinguishing "resource doesn't exist" from other errors,
// e.g. when checking for a rotated client secret that hasn't been created yet.
func IsNotFound(err error) bool {
	var ae *apiError
	if ok := errorAs(err, &ae); ok {
		return ae.statusCode == http.StatusNotFound
	}
	return false
}

// errorAs is a thin wrapper around errors.As to allow testing with the
// unexported apiError type. Production code uses the standard library.
var errorAs = func(err error, target interface{}) bool {
	return stderrors.As(err, target)
}
