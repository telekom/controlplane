// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceIDFromResponse_ValidLocation(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Location", "https://keycloak.example.com/auth/admin/realms/myrealm/clients/abc-123-def")

	id, err := resourceIDFromResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, "abc-123-def", id)
}

func TestResourceIDFromResponse_TrailingSlash(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Location", "https://keycloak.example.com/auth/admin/realms/myrealm/clients/abc-123/")

	id, err := resourceIDFromResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, "abc-123", id)
}

func TestResourceIDFromResponse_MissingHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}

	_, err := resourceIDFromResponse(resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Location header")
}

func TestResourceIDFromResponse_EmptyPath(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Location", "https://keycloak.example.com/")

	// The last segment after trimming "/" is "keycloak.example.com" — which is not empty,
	// but this tests the edge case. The path "/" after trim becomes "", split gives ["", ""].
	// Actually path is "/" -> trimRight "/" -> "" -> split "" -> [""] -> last = "" -> error.
	_, err := resourceIDFromResponse(resp)
	require.Error(t, err)
}
