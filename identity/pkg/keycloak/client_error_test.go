// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type MockApiResponse struct {
	statusCode int
}

func (m *MockApiResponse) StatusCode() int {
	return m.statusCode
}

func TestStatusCodeIsOk(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusOK}
	err := CheckStatusCode(response, http.StatusOK)
	assert.Nil(t, err)
}

func TestStatusCodeIsNotFound(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusNotFound}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak client error", err.Error())
	assert.False(t, err.Retriable())
	// ctrlerrors-compatible interface
	assert.False(t, err.IsRetryable())
	assert.True(t, err.IsBlocked())
	assert.Equal(t, time.Duration(0), err.RetryDelay())
}

func TestStatusCodeIsInternalServerError(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusInternalServerError}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak server error", err.Error())
	assert.True(t, err.Retriable())
	// ctrlerrors-compatible interface
	assert.True(t, err.IsRetryable())
	assert.False(t, err.IsBlocked())
	assert.Equal(t, time.Duration(0), err.RetryDelay())
}

func TestStatusCodeIsBadRequest(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusBadRequest}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak client error", err.Error())
	assert.False(t, err.Retriable())
	// ctrlerrors-compatible interface
	assert.False(t, err.IsRetryable())
	assert.True(t, err.IsBlocked())
	assert.Equal(t, time.Duration(0), err.RetryDelay())
}

func TestStatusCodeIsServiceUnavailable(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusServiceUnavailable}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak server error", err.Error())
	assert.True(t, err.Retriable())
	// ctrlerrors-compatible interface
	assert.True(t, err.IsRetryable())
	assert.False(t, err.IsBlocked())
	assert.Equal(t, time.Duration(0), err.RetryDelay())
}

func TestStatusCodeIsTooManyRequests(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusTooManyRequests}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak rate limit error", err.Error())
	assert.True(t, err.Retriable())
	// ctrlerrors-compatible interface
	assert.True(t, err.IsRetryable())
	assert.False(t, err.IsBlocked())
	assert.Equal(t, 3*time.Second, err.RetryDelay())
}

func TestStatusCodeMultipleAcceptable(t *testing.T) {
	// Verify that multiple acceptable status codes work
	response := &MockApiResponse{statusCode: http.StatusNoContent}
	err := CheckStatusCode(response, http.StatusOK, http.StatusNoContent)
	assert.Nil(t, err)
}

func TestApiErrorImplementsErrorInterface(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusForbidden}
	apiErr := CheckStatusCode(response, http.StatusOK)
	// Verify it satisfies the standard error interface
	var stdErr error = apiErr
	assert.NotNil(t, stdErr)
	assert.Contains(t, stdErr.Error(), "Keycloak")
}
