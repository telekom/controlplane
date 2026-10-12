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

// Categories of RequestError. They are fixed values so that diagnostics never
// carry content from the remote side or the request URL.
const (
	RequestErrorAuthentication = "authentication"
	RequestErrorTimeout        = "timeout"
	RequestErrorCanceled       = "canceled"
	RequestErrorConnection     = "connection"
	RequestErrorInvalid        = "invalid request"
)

var _ error = &RequestError{}

// RequestError is returned when a request could not be completed.
// Error() only contains the method, resource and a fixed category; the
// wrapped error may contain URLs or remote response content and is only
// available through errors.Is/errors.As.
type RequestError struct {
	Method   string
	Resource string
	Category string
	// StatusCode is the HTTP status of the token endpoint for authentication failures, if known.
	StatusCode int
	err        error
}

func (e *RequestError) Error() string {
	msg := fmt.Sprintf("%s request for %s failed: %s error", e.Method, e.Resource, e.Category)
	if e.StatusCode != 0 {
		msg += fmt.Sprintf(" (HTTP status %d)", e.StatusCode)
	}
	return msg
}

func (e *RequestError) Unwrap() error {
	return e.err
}

var _ error = &ResponseDecodeError{}

// ResponseDecodeError is returned when a successful response body cannot be
// decoded. Decoder and reader errors may quote response content, so Error()
// only contains the resource; the wrapped error is available through
// errors.Is/errors.As.
type ResponseDecodeError struct {
	Resource string
	err      error
}

func (e *ResponseDecodeError) Error() string {
	return fmt.Sprintf("failed to parse %s response: invalid response body", e.Resource)
}

func (e *ResponseDecodeError) Unwrap() error {
	return e.err
}

// DecodeResponse decodes a JSON response body into v and returns a
// ResponseDecodeError on failure.
func DecodeResponse(resource string, body io.Reader, v any) error {
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return &ResponseDecodeError{Resource: resource, err: err}
	}
	return nil
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
