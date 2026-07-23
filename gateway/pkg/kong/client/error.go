// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"time"

	"golang.org/x/oauth2"

	"github.com/telekom/controlplane/common-server/pkg/client"
)

type ApiResponse interface {
	StatusCode() int
}

// ApiError represents an error from the Kong Admin API.
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

// Retriable is the original Kong-specific method, kept for backward compatibility.
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

// CheckStatusCode classifies a Kong Admin API response into an ApiError.
// It returns nil when the response status code is in okStatusCodes.
func CheckStatusCode(res ApiResponse, okStatusCodes ...int) ApiError {
	if slices.Contains(okStatusCodes, res.StatusCode()) {
		return nil
	}

	if res.StatusCode() == http.StatusTooManyRequests {
		return &apiError{
			statusCode:   res.StatusCode(),
			message:      fmt.Sprintf("Kong rate limit error (%d)", res.StatusCode()),
			retryAllowed: true,
			retryDelay:   3 * time.Second,
		}
	}

	if res.StatusCode() >= http.StatusInternalServerError {
		return &apiError{
			statusCode:   res.StatusCode(),
			message:      fmt.Sprintf("Kong server error (%d)", res.StatusCode()),
			retryAllowed: true,
		}
	}

	return &apiError{
		statusCode:   res.StatusCode(),
		message:      fmt.Sprintf("Kong client error (%d)", res.StatusCode()),
		retryAllowed: false,
	}
}

// CheckHTTPStatus classifies a raw HTTP status code into an ApiError.
// It returns nil when statusCode is in okStatusCodes.
// This is the lower-level sibling of CheckStatusCode for callers that do not
// have an ApiResponse (e.g. oauth2.RetrieveError).
func CheckHTTPStatus(statusCode int, okStatusCodes ...int) ApiError {
	if slices.Contains(okStatusCodes, statusCode) {
		return nil
	}

	if statusCode == http.StatusTooManyRequests {
		return &apiError{
			statusCode:   statusCode,
			message:      fmt.Sprintf("Kong rate limit error (%d)", statusCode),
			retryAllowed: true,
			retryDelay:   3 * time.Second,
		}
	}

	if statusCode >= http.StatusInternalServerError {
		return &apiError{
			statusCode:   statusCode,
			message:      fmt.Sprintf("Kong server error (%d)", statusCode),
			retryAllowed: true,
		}
	}

	return &apiError{
		statusCode:   statusCode,
		message:      fmt.Sprintf("Kong client error (%d)", statusCode),
		retryAllowed: false,
	}
}

// IsNotFound returns true if the error is an ApiError with a 404 status code.
func IsNotFound(err error) bool {
	var ae *apiError
	if ok := errors.As(err, &ae); ok {
		return ae.statusCode == http.StatusNotFound
	}
	return false
}

func WrapApiResponse(res *http.Response) ApiResponse {
	return &responseWrapper{
		response: res,
	}
}

type responseWrapper struct {
	response *http.Response
}

func (r *responseWrapper) StatusCode() int {
	if r.response == nil {
		return 0
	}
	return r.response.StatusCode
}

// HandleClientError classifies transport-level errors from the HTTP client so
// that ctrlerrors.HandleError can route them correctly.
//
// - nil → nil
// - Already an *apiError (e.g. from CheckStatusCode) → returned as-is
// - context.Canceled / context.DeadlineExceeded → returned as-is (controller framework handles these)
// - oauth2.RetrieveError → classified by the token endpoint's HTTP status (429/5xx retryable, 4xx blocked)
// - net.Error (connection refused, DNS, TLS, timeout) → wrapped as a retryable apiError
// - Anything else → returned as-is
func HandleClientError(err error) error {
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*apiError](err); ok { //nolint:errcheck // Only the type match is needed.
		return err
	}

	// Context cancellation is handled by the controller framework, not here.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// OAuth2 token endpoint errors from clientcredentials — classify by
	// the token endpoint's HTTP status so 429/5xx retry and 4xx block.
	if oauth2Err, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return client.HandleError(oauth2Err.Response.StatusCode, fmt.Sprintf("OAuth2 token endpoint error: %s", oauth2Err.Error()))
	}

	// ponytail: net.Error covers all transient transport failures (timeout,
	// connection refused, DNS, TLS). Classify as retryable so the reconciler
	// requeues instead of blocking.
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return &apiError{
			statusCode:   0,
			message:      fmt.Sprintf("transport error: %s", netErr.Error()),
			retryAllowed: true,
		}
	}

	return err
}
