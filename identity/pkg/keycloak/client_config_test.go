// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	adminUrl              = "https://example.com/auth/admin/realms/"
	adminUrlWithoutAdmin  = "https://example.com/auth/"
	adminUrlWithoutRealms = "https://example.com/auth/admin/"
	realmName             = "test-realm"
)

func TestAdminConsoleUrlFromAdminUrlAndRealmName(t *testing.T) {
	expected := "https://example.com/auth/admin/master/console/#/test-realm"
	result := DetermineAdminConsoleUrlFrom(adminUrl, realmName)
	assert.Equal(t, expected, result)
}

func TestAdminConsoleUrlFromAdminUrlWithoutRealms(t *testing.T) {
	expected := "https://example.com/auth/admin/master/console/#/test-realm"
	result := DetermineAdminConsoleUrlFrom(adminUrlWithoutRealms, realmName)
	assert.Equal(t, expected, result)
}

func TestIssuerUrlFromAdminUrlAndRealmName(t *testing.T) {
	expected := "https://example.com/auth/realms/test-realm"
	result := DetermineIssuerUrlFrom(adminUrl, realmName)
	assert.Equal(t, expected, result)
}

func TestIssuerUrlFromAdminUrlWithoutAdmin(t *testing.T) {
	expected := "https://example.com/auth/realms/test-realm"
	result := DetermineIssuerUrlFrom(adminUrlWithoutAdmin, realmName)
	assert.Equal(t, expected, result)
}

func TestAdminTokenUrlFromAdminUrlAndRealmName(t *testing.T) {
	expected := "https://example.com/auth/realms/test-realm/protocol/openid-connect/token"
	result := DetermineAdminTokenUrlFrom(adminUrl, realmName)
	assert.Equal(t, expected, result)
}
