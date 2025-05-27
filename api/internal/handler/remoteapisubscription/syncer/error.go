// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package syncer

import "slices"

type ApiResponse interface {
	StatusCode() int
}

type ApiError interface {
	error
	Retriable() bool
	withBody([]byte) ApiError
}

type apiError struct {
	StatusCode   int
	Message      string
	RetryAllowed bool
}

func (e *apiError) Error() string {
	return e.Message
}

func (e *apiError) Retriable() bool {
	return e.RetryAllowed
}

func (e *apiError) withBody(body []byte) ApiError {
	e.Message = e.Message + ": " + string(body)
	return e
}

func CheckStatusCode(res ApiResponse, okStatusCodes ...int) ApiError {
	if slices.Contains(okStatusCodes, res.StatusCode()) {
		return nil
	}

	if res.StatusCode() >= 500 {
		return &apiError{
			StatusCode:   res.StatusCode(),
			Message:      "server error",
			RetryAllowed: true,
		}
	}

	return &apiError{
		StatusCode:   res.StatusCode(),
		Message:      "client error",
		RetryAllowed: false,
	}
}
