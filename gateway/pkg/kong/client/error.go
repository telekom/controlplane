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

// HandleClientError classifies transport-level errors from the HTTP client so
// that ctrlerrors.HandleError can route them correctly.
//
// - nil → nil
// - Already an *apiError (e.g. from CheckStatusCode) → returned as-is
// - context.Canceled / context.DeadlineExceeded → returned as-is (controller framework handles these)
// - oauth2.RetrieveError → see classifyTokenError
// - net.Error (connection refused, DNS, TLS, timeout) → wrapped as a retryable apiError
// - Anything else → returned as-is
func HandleClientError(err error) error {
	if err == nil {
		return nil
	}

	var alreadyClassified *apiError
	if errors.As(err, &alreadyClassified) {
		return err
	}

	// Context cancellation is handled by the controller framework, not here.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if oauth2Err, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return classifyTokenError(oauth2Err)
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

// classifyTokenError blocks on 4xx token endpoint responses other than 429.
// Every other failure is retryable without a fixed delay, so the controller
// backs off exponentially while the identity provider is unavailable.
func classifyTokenError(err *oauth2.RetrieveError) error {
	msg := fmt.Sprintf("OAuth2 token endpoint error: %s", err.Error())
	statusCode := err.Response.StatusCode
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError && statusCode != http.StatusTooManyRequests {
		return client.HandleError(statusCode, msg)
	}
	return &apiError{
		statusCode:   statusCode,
		message:      msg,
		retryAllowed: true,
	}
}
