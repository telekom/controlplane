// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"testing"

	"github.com/stretchr/testify/assert"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

type MockAdminConfig struct {
	endpointUrl  string
	issuerUrl    string
	tokenUrl     string
	clientId     string
	clientSecret string
	username     string
	password     string
}

func (m MockAdminConfig) EndpointUrl() string {
	return m.endpointUrl
}

func (m MockAdminConfig) IssuerUrl() string {
	return m.issuerUrl
}

func (m MockAdminConfig) TokenUrl() string {
	return m.tokenUrl
}

func (m MockAdminConfig) ClientId() string {
	return m.clientId
}

func (m MockAdminConfig) ClientSecret() string {
	return m.clientSecret
}

func (m MockAdminConfig) Username() string {
	return m.username
}

func (m MockAdminConfig) Password() string {
	return m.password
}

func TestOauth2ClientCreationFailureInvalidTokenUrl(t *testing.T) {
	config := &MockAdminConfig{
		tokenUrl: "://invalid-url",
	}
	client, err := newOauth2Client(config)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestOauth2ClientCreationFailureInvalidCredentials(t *testing.T) {
	config := &MockAdminConfig{
		tokenUrl: "https://example.com/auth/realms/test-realm/protocol/openid-connect/token",
		username: "invalid-username",
		password: "invalid-password",
	}
	client, err := newOauth2Client(config)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestClientForRealmCreationHostError(t *testing.T) {
	realmStatus := identityv1.RealmStatus{
		AdminUrl:      "https://example.com/auth/admin/realms/",
		IssuerUrl:     "https://example.com/auth/realms/test-realm",
		AdminTokenUrl: "https://example.com/auth/realms/test-realm/protocol/openid-connect/token",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
	}
	client, err := GetClientForRealm(realmStatus)
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestClientForRealmCreationFailure(t *testing.T) {
	realmStatus := identityv1.RealmStatus{
		AdminUrl:      "https://example.com/auth/admin/realms/",
		IssuerUrl:     "https://example.com/auth/realms/test-realm",
		AdminTokenUrl: "://invalid-url",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
	}
	client, err := GetClientForRealm(realmStatus)
	assert.Error(t, err)
	assert.Nil(t, client)
}
