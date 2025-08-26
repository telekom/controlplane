// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmderrors

import "fmt"

type Error struct {
	Message    string `json:"message"`
	Reason     string `json:"reason,omitempty"`
	Code       int    `json:"code,omitempty"`
	ApiVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) WithApiVersion(apiVersion string) *Error {
	e.ApiVersion = apiVersion
	return e
}

func (e *Error) WithKind(kind string) *Error {
	e.Kind = kind
	return e
}

func FileNotFound(filePath string) *Error {
	return &Error{
		Message: fmt.Sprintf("file not found: %s", filePath),
		Reason:  "FileNotFound",
		Code:    404,
	}
}
