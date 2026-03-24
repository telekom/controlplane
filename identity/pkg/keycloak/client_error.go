// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
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
	if slices.Contains(okStatusCodes, res.StatusCode()) {
		return nil
	}

	if res.StatusCode() == http.StatusTooManyRequests {
		return &apiError{
			statusCode:   res.StatusCode(),
			message:      "Keycloak rate limit error",
			retryAllowed: true,
			retryDelay:   3 * time.Second,
		}
	}

	if res.StatusCode() >= http.StatusInternalServerError {
		return &apiError{
			statusCode:   res.StatusCode(),
			message:      "Keycloak server error",
			retryAllowed: true,
		}
	}

	return &apiError{
		statusCode:   res.StatusCode(),
		message:      "Keycloak client error",
		retryAllowed: false,
	}
}
