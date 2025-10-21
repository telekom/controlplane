// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"fmt"
	"slices"
	"time"
)

// HttpError implement the common/pkg/errors/ctrlerrors.Error interface
// However, we do not want to introduce a dependency from common-server/pkg/client to common/pkg/errors
type HttpError struct {
	msg        string
	retryable  bool
	retryDelay time.Duration
	blocked    bool
}

func (e *HttpError) Error() string {
	return e.msg
}

func (e *HttpError) IsBlocked() bool {
	return e.blocked
}

func (e *HttpError) IsRetryable() bool {
	return e.retryable
}

func (e *HttpError) RetryDelay() time.Duration {
	return e.retryDelay
}

func BlockedErrorf(format string, a ...any) *HttpError {
	return &HttpError{
		msg:     fmt.Sprintf(format, a...),
		blocked: true,
	}
}

func RetryableErrorf(format string, a ...any) *HttpError {
	return &HttpError{
		msg:       fmt.Sprintf(format, a...),
		retryable: true,
	}
}

func RetryableWithDelayErrorf(delay time.Duration, format string, a ...any) *HttpError {
	return &HttpError{
		msg:        fmt.Sprintf(format, a...),
		retryable:  true,
		retryDelay: delay,
	}
}

var (
	RetryDelay = 5 * time.Second
)

func HandleError(httpStatus int, msg string, okStatusCodes ...int) error {
	if len(okStatusCodes) == 0 {
		okStatusCodes = []int{200, 201, 202, 204}
	}
	if slices.Contains(okStatusCodes, httpStatus) {
		return nil
	}
	switch httpStatus {
	case 400:
		return BlockedErrorf("bad request error (%d): %s", httpStatus, msg)
	case 409, 500, 502, 504:
		return RetryableErrorf("server error (%d): %s", httpStatus, msg)
	case 429, 503:
		return RetryableWithDelayErrorf(RetryDelay, "rate limit error (%d): %s", httpStatus, msg)
	default:
		return fmt.Errorf("unknown error (%d): %s", httpStatus, msg)
	}
}
