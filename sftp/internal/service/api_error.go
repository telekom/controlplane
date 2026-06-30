// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
)

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
	case statusCode == http.StatusBadRequest,
		statusCode == http.StatusUnauthorized,
		statusCode == http.StatusForbidden,
		statusCode == http.StatusNotFound:
		return ctrlerrors.BlockedErrorf("%s", errMessage)
	case statusCode >= http.StatusInternalServerError:
		return ctrlerrors.RetryableErrorf("%s", errMessage)
	default:
		return ctrlerrors.RetryableErrorf("%s", errMessage)
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
