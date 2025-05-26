// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"net/http"
	"slices"
)

type ApiResponse interface {
	StatusCode() int
}

type ApiError interface {
	error
	Retriable() bool
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

func CheckStatusCode(res ApiResponse, okStatusCodes ...int) ApiError {
	if slices.Contains(okStatusCodes, res.StatusCode()) {
		return nil
	}

	if res.StatusCode() >= http.StatusInternalServerError {
		return &apiError{
			StatusCode:   res.StatusCode(),
			Message:      "Keycloak server error",
			RetryAllowed: true,
		}
	}

	return &apiError{
		StatusCode:   res.StatusCode(),
		Message:      "Keycloak client error",
		RetryAllowed: false,
	}
}
