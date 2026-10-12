// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/telekom/controlplane/identity/pkg/api"
)

// unauthorizedRetiringDoer wraps the authenticated Admin API HTTP client.
// When the Admin API answers 401, the doer retires itself, invokes
// onUnauthorized (once) and returns a retryable error instead of the response.
// A retired doer rejects all further requests without sending them, so
// holders of a stale service cannot keep reusing rejected credentials.
// The rejected request is never replayed; recovery happens on the next
// reconciliation, which obtains a freshly authenticated service.
type unauthorizedRetiringDoer struct {
	next           api.HttpRequestDoer
	retired        atomic.Bool
	onUnauthorized func()
}

func newUnauthorizedRetiringDoer(next api.HttpRequestDoer, onUnauthorized func()) *unauthorizedRetiringDoer {
	return &unauthorizedRetiringDoer{next: next, onUnauthorized: onUnauthorized}
}

func (d *unauthorizedRetiringDoer) Do(req *http.Request) (*http.Response, error) {
	if d.retired.Load() {
		return nil, retiredClientError()
	}

	res, err := d.next.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusUnauthorized {
		return res, nil
	}

	apiErr := &apiError{
		statusCode:   http.StatusUnauthorized,
		message:      "Keycloak admin API rejected credentials (401), client retired for re-authentication",
		retryAllowed: true,
	}
	if d.retired.CompareAndSwap(false, true) && d.onUnauthorized != nil {
		d.onUnauthorized()
	}
	if closeErr := res.Body.Close(); closeErr != nil {
		return nil, errors.Join(apiErr, fmt.Errorf("closing rejected response body: %w", closeErr))
	}
	return nil, apiErr
}

func retiredClientError() error {
	return &apiError{
		statusCode:   http.StatusUnauthorized,
		message:      "Keycloak admin client retired after 401, awaiting re-authentication",
		retryAllowed: true,
	}
}
