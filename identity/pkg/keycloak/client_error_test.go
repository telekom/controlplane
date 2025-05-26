// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"net/http"
	"testing"

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
}

func TestStatusCodeIsInternalServerError(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusInternalServerError}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak server error", err.Error())
	assert.True(t, err.Retriable())
}

func TestStatusCodeIsBadRequest(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusBadRequest}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak client error", err.Error())
	assert.False(t, err.Retriable())
}

func TestStatusCodeIsServiceUnavailable(t *testing.T) {
	response := &MockApiResponse{statusCode: http.StatusServiceUnavailable}
	err := CheckStatusCode(response, http.StatusOK)
	assert.NotNil(t, err)
	assert.Equal(t, "Keycloak server error", err.Error())
	assert.True(t, err.Retriable())
}
