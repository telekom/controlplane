// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"errors"
	"fmt"

	"golang.org/x/oauth2"
)

// classifyOAuth2Error converts an oauth2 token-endpoint error into an
// apiError with the correct retry/blocked classification. This ensures
// errors from the token layer integrate with ctrlerrors.HandleError via
// the same duck-typed interfaces as Keycloak API errors.
func classifyOAuth2Error(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		apiErr := CheckHTTPStatus(re.Response.StatusCode)
		if apiErr != nil {
			return apiErr
		}
	}

	// Network, DNS, timeout, or other non-HTTP errors — retryable.
	return &apiError{
		message:      fmt.Sprintf("token endpoint error: %s", err.Error()),
		retryAllowed: true,
	}
}
