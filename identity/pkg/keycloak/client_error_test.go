// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"fmt"
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
	assert.Equal(t, "Keycloak client error (404)", err.Error())
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
	assert.Equal(t, "Keycloak server error (500)", err.Error())
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
	assert.Equal(t, "Keycloak client error (400)", err.Error())
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
	assert.Equal(t, "Keycloak server error (503)", err.Error())
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
	assert.Equal(t, "Keycloak rate limit error (429)", err.Error())
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

func TestIsNotFound_True(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusNotFound}
	err := CheckStatusCode(response, http.StatusOK)
	assert.True(t, IsNotFound(err))
}

func TestIsNotFound_FalseForOtherClientError(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusBadRequest}
	err := CheckStatusCode(response, http.StatusOK)
	assert.False(t, IsNotFound(err))
}

func TestIsNotFound_FalseForServerError(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusInternalServerError}
	err := CheckStatusCode(response, http.StatusOK)
	assert.False(t, IsNotFound(err))
}

func TestIsNotFound_FalseForNilError(t *testing.T) {
	assert.False(t, IsNotFound(nil))
}

func TestIsNotFound_FalseForNonApiError(t *testing.T) {
	err := fmt.Errorf("some random error")
	assert.False(t, IsNotFound(err))
}

func TestCheckHTTPStatus_Ok(t *testing.T) {
	err := CheckHTTPStatus(200, 200)
	assert.Nil(t, err)
}

func TestCheckHTTPStatus_ClientError(t *testing.T) {
	err := CheckHTTPStatus(400, 200)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak client error (400)", err.Error())
	assert.True(t, err.IsBlocked())
	assert.False(t, err.IsRetryable())
}

func TestCheckHTTPStatus_ServerError(t *testing.T) {
	err := CheckHTTPStatus(502, 200)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak server error (502)", err.Error())
	assert.False(t, err.IsBlocked())
	assert.True(t, err.IsRetryable())
}

func TestCheckHTTPStatus_RateLimit(t *testing.T) {
	err := CheckHTTPStatus(429, 200)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak rate limit error (429)", err.Error())
	assert.True(t, err.IsRetryable())
	assert.Equal(t, 3*time.Second, err.RetryDelay())
}

func TestCheckHTTPStatus_MultipleOkCodes(t *testing.T) {
	assert.Nil(t, CheckHTTPStatus(204, 200, 204))
	assert.NotNil(t, CheckHTTPStatus(201, 200, 204))
}
