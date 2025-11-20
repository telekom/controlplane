// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

type FieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

var _ error = &ApiError{}

type ApiError struct {
	Type     string       `json:"type"`
	Status   int          `json:"status"`
	Title    string       `json:"title"`
	Detail   string       `json:"detail"`
	Instance string       `json:"instance"`
	Fields   []FieldError `json:"fields,omitempty"`
}

func (e *ApiError) Error() string {
	return e.Title + ": " + e.Detail
}

func IsValidationError(err error) bool {
	if apiErr, ok := AsApiError(err); ok {
		return apiErr.Type == "ValidationError"
	}
	return false
}

func AsApiError(err error) (*ApiError, bool) {
	var apiErr *ApiError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

func PrintTo(err error, w io.Writer, format string) {
	switch format {
	case "json":
		PrintJsonTo(err, w)
	default:
		PrintTextTo(err, w)
	}
}

func PrintTextTo(err error, w io.Writer) {
	apiErr, ok := AsApiError(err)
	if !ok {
		apiErr = &ApiError{
			Type:   "InternalError",
			Status: 500,
			Title:  "Internal Server Error",
			Detail: err.Error(),
		}
	}

	_, _ = fmt.Fprintf(w, "❌ Error\n--------\n")
	_, _ = fmt.Fprintf(w, "Type: %s\nStatus: %d\nTitle: %s\nDetail: %s\n",
		apiErr.Type, apiErr.Status, apiErr.Title, apiErr.Detail)
	if apiErr.Instance != "" {
		_, _ = fmt.Fprintf(w, "Instance: %s\n", apiErr.Instance)
	}

	if len(apiErr.Fields) > 0 {
		_, _ = fmt.Fprintln(w, "Fields:")
		for _, field := range apiErr.Fields {
			_, _ = fmt.Fprintf(w, "  Field: %s\n    Detail: %s\n", field.Field, field.Detail)
		}
	}
	_, _ = fmt.Fprintln(w)
}

func PrintJsonTo(err error, w io.Writer) {
	apiErr, ok := AsApiError(err)
	if !ok {
		apiErr = &ApiError{
			Type:   "InternalError",
			Status: 500,
			Title:  "Internal Server Error",
			Detail: err.Error(),
		}
	}

	data, _ := json.MarshalIndent(apiErr, "", "  ")
	_, _ = w.Write(data)
	_, _ = fmt.Fprintln(w)
}
