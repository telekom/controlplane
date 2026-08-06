// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

var ErrNotFound = errors.New("not found")

func firstAPIError(apiErrors ...*ApiErrorResponse) *ApiErrorResponse {
	for _, apiErr := range apiErrors {
		if apiErr != nil {
			return apiErr
		}
	}
	return nil
}

func handleAPIError(operation string, statusCode int, body []byte, apiErr *ApiErrorResponse) error {
	message := apiErrorMessage(apiErr)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	errMessage := fmt.Sprintf("SFTP Tardis API returned %d while trying to %s: %s", statusCode, operation, message)
	switch {
	case statusCode == http.StatusBadRequest:
		return ctrlerrors.BlockedErrorf("%s", errMessage)
	case statusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, errMessage)
	case statusCode == http.StatusUnauthorized,
		statusCode == http.StatusForbidden:
		return errors.New(errMessage)
	case statusCode >= http.StatusInternalServerError:
		return errors.New(errMessage)
	default:
		return errors.New(errMessage)
	}
}

func apiErrorMessage(apiErr *ApiErrorResponse) string {
	if apiErr == nil {
		return ""
	}

	parts := make([]string, 0, 2)
	if apiErr.Title != nil && *apiErr.Title != "" {
		parts = append(parts, *apiErr.Title)
	}
	if apiErr.Detail != nil && *apiErr.Detail != "" {
		parts = append(parts, *apiErr.Detail)
	}
	if apiErr.Errors != nil {
		for _, detail := range *apiErr.Errors {
			switch {
			case detail.FieldName != nil && *detail.FieldName != "" && detail.Error != nil && *detail.Error != "":
				parts = append(parts, fmt.Sprintf("%s: %s", *detail.FieldName, *detail.Error))
			case detail.Error != nil && *detail.Error != "":
				parts = append(parts, *detail.Error)
			}
		}
	}

	return strings.Join(parts, "; ")
}
