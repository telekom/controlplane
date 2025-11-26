// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
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

	fmt.Fprintf(w, "❌ Error\n--------\n")
	fmt.Fprintf(w, "Type: %s\nStatus: %d\nTitle: %s\nDetail: %s\n",
		apiErr.Type, apiErr.Status, apiErr.Title, apiErr.Detail)
	if apiErr.Instance != "" {
		fmt.Fprintf(w, "Instance: %s\n", apiErr.Instance)
	}

	if len(apiErr.Fields) > 0 {
		fmt.Fprintln(w, "Fields:")
		for _, field := range apiErr.Fields {
			fmt.Fprintf(w, "  Field: %s\n    Detail: %s\n", field.Field, field.Detail)
		}
	}
	fmt.Fprintln(w)
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
	w.Write(data)
	fmt.Fprintln(w)
}

func ValidationError(obj types.Object, fields ...FieldError) *ApiError {
	kind := obj.GetKind()
	if kind == "" {
		kind = "Object"
	}
	detail := fmt.Sprintf("%s failed validation", kind)
	filename := obj.GetProperty("filename")
	if filenameStr, ok := filename.(string); ok && filenameStr != "" {
		detail = fmt.Sprintf("%s defined in file %q failed validation", kind, filenameStr)
	}

	return &ApiError{
		Type:   "ValidationError",
		Status: 400,
		Title:  fmt.Sprintf("Failed to validate %s %q", kind, obj.GetName()),
		Detail: detail,
		Fields: fields,
	}
}
