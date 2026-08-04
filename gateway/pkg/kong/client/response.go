// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// summarizeBody renders a Kong error body for an error message. Kong describes
// a rejected request in "name" and "message" but repeats the offending request
// under "fields", which for an authentication plugin carries the credentials
// themselves, so only the description is kept. These errors travel up to the
// reconciler and are logged there.
func summarizeBody(body []byte) string {
	var described struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &described); err != nil {
		return "<unreadable body>"
	}
	switch {
	case described.Name != "" && described.Message != "":
		return described.Name + ": " + described.Message
	case described.Message != "":
		return described.Message
	case described.Name != "":
		return described.Name
	default:
		return "<no description>"
	}
}

// readResult carries the parts of a generated Kong response that the entities
// need. The generated types expose Body and JSON200 as fields rather than
// methods, so no interface can reach them and each entity unpacks them itself.
type readResult[T any] struct {
	status int
	body   []byte
	value  *T
}

// statusCode adapts a bare status code to ApiResponse.
type statusCode int

func (s statusCode) StatusCode() int { return int(s) }

// readOne interprets a GET response. A 404 reports the entity as absent.
func readOne[T any](name string, r readResult[T]) (*T, bool, error) {
	if err := CheckStatusCode(statusCode(r.status), http.StatusOK, http.StatusNotFound); err != nil {
		return nil, false, fmt.Errorf("failed to get %s (%d): %s: %w", name, r.status, summarizeBody(r.body), err)
	}
	if r.status == http.StatusNotFound {
		return nil, false, nil
	}
	if r.value == nil {
		return nil, false, fmt.Errorf("%s response body is missing", name)
	}
	return r.value, true, nil
}

// writeOne interprets a write response.
func writeOne[T any](name string, r readResult[T], okCodes ...int) (*T, error) {
	if err := CheckStatusCode(statusCode(r.status), okCodes...); err != nil {
		return nil, fmt.Errorf("failed to write %s (%d): %s: %w", name, r.status, summarizeBody(r.body), err)
	}
	if r.value == nil {
		return nil, fmt.Errorf("%s response body is missing", name)
	}
	return r.value, nil
}
