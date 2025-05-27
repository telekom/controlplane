// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func TestObfuscateRealmMapsCorrectly(t *testing.T) {
	realmStatus := identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}

	obfuscatedStatus := ObfuscateRealm(realmStatus)

	assert.Equal(t, "https://issuer.example.com", obfuscatedStatus.IssuerUrl)
	assert.Equal(t, "admin-client-id", obfuscatedStatus.AdminClientId)
	assert.Equal(t, "****", obfuscatedStatus.AdminUserName)
	assert.Equal(t, "****", obfuscatedStatus.AdminPassword)
	assert.Equal(t, "https://admin.example.com", obfuscatedStatus.AdminUrl)
	assert.Equal(t, "https://admin.example.com/token", obfuscatedStatus.AdminTokenUrl)
}

func TestValidateRealmStatusReturnsErrorWhenStatusIsNil(t *testing.T) {
	err := ValidateRealmStatus(nil)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus is nil", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenIssuerUrlIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.IssuerUrl is empty", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenAdminClientIdIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.AdminClientId is empty", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenAdminUserNameIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.AdminUserName is empty", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenAdminPasswordIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.AdminPassword is empty", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenAdminUrlIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.AdminUrl is empty", err.Error())
}

func TestValidateRealmStatusReturnsErrorWhenAdminTokenUrlIsEmpty(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.Error(t, err)
	assert.Equal(t, "realmStatus.AdminTokenUrl is empty", err.Error())
}

func TestValidateRealmStatusReturnsNilWhenAllFieldsAreValid(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	err := ValidateRealmStatus(realmStatus)
	assert.NoError(t, err)
}
