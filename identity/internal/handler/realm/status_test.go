// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

func TestMapToRealmStatusMapsCorrectly(t *testing.T) {
	identityProvider := &identityv1.IdentityProvider{
		Spec: identityv1.IdentityProviderSpec{
			AdminUrl:      "https://admin.example.com",
			AdminClientId: "admin-client-id",
			AdminUserName: "admin-username",
			AdminPassword: "admin-password",
		},
		Status: identityv1.IdentityProviderStatus{
			AdminUrl:      "https://admin.example.com",
			AdminTokenUrl: "https://admin.example.com/token",
		},
	}
	realmName := "test-realm"

	realmStatus := MapToRealmStatus(identityProvider, realmName)

	assert.Equal(t, keycloak.DetermineIssuerUrlFrom(identityProvider.Spec.AdminUrl, realmName), realmStatus.IssuerUrl)
	assert.Equal(t, identityProvider.Spec.AdminClientId, realmStatus.AdminClientId)
	assert.Equal(t, identityProvider.Spec.AdminUserName, realmStatus.AdminUserName)
	assert.Equal(t, identityProvider.Spec.AdminPassword, realmStatus.AdminPassword)

	assert.Equal(t, identityProvider.Status.AdminUrl, realmStatus.AdminUrl)
	assert.Equal(t, identityProvider.Status.AdminTokenUrl, realmStatus.AdminTokenUrl)
}

func TestSetStatusBlockedSetsRealmStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	realm := &identityv1.Realm{}

	SetStatusBlocked(currentStatus, realm)

	assert.Equal(t, currentStatus.IssuerUrl, realm.Status.IssuerUrl)
	assert.Equal(t, currentStatus.AdminClientId, realm.Status.AdminClientId)
	assert.Equal(t, currentStatus.AdminUserName, realm.Status.AdminUserName)
	assert.Equal(t, currentStatus.AdminPassword, realm.Status.AdminPassword)
	assert.Equal(t, currentStatus.AdminUrl, realm.Status.AdminUrl)
	assert.Equal(t, currentStatus.AdminTokenUrl, realm.Status.AdminTokenUrl)
	assert.True(t, HasConditions(t, realm, []v1.Condition{blockedCondition, blockedNotReadyCondition}))
}

func TestSetStatusProcessingSetsRealmStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	realm := &identityv1.Realm{}

	SetStatusProcessing(currentStatus, realm)

	assert.Equal(t, currentStatus.IssuerUrl, realm.Status.IssuerUrl)
	assert.Equal(t, currentStatus.AdminClientId, realm.Status.AdminClientId)
	assert.Equal(t, currentStatus.AdminUserName, realm.Status.AdminUserName)
	assert.Equal(t, currentStatus.AdminPassword, realm.Status.AdminPassword)
	assert.Equal(t, currentStatus.AdminUrl, realm.Status.AdminUrl)
	assert.Equal(t, currentStatus.AdminTokenUrl, realm.Status.AdminTokenUrl)
	assert.True(t, HasConditions(t, realm, []v1.Condition{processingCondition, processingNotReadyCondition}))
}

func TestSetStatusWaitingSetsRealmStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	realm := &identityv1.Realm{}

	SetStatusWaiting(currentStatus, realm)

	assert.Equal(t, currentStatus.IssuerUrl, realm.Status.IssuerUrl)
	assert.Equal(t, currentStatus.AdminClientId, realm.Status.AdminClientId)
	assert.Equal(t, currentStatus.AdminUserName, realm.Status.AdminUserName)
	assert.Equal(t, currentStatus.AdminPassword, realm.Status.AdminPassword)
	assert.Equal(t, currentStatus.AdminUrl, realm.Status.AdminUrl)
	assert.Equal(t, currentStatus.AdminTokenUrl, realm.Status.AdminTokenUrl)
	assert.True(t, HasConditions(t, realm, []v1.Condition{waitingCondition, waitingNotReadyCondition}))
}

func TestSetStatusReadySetsRealmStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	realm := &identityv1.Realm{}

	SetStatusReady(currentStatus, realm)

	assert.Equal(t, currentStatus.IssuerUrl, realm.Status.IssuerUrl)
	assert.Equal(t, currentStatus.AdminClientId, realm.Status.AdminClientId)
	assert.Equal(t, currentStatus.AdminUserName, realm.Status.AdminUserName)
	assert.Equal(t, currentStatus.AdminPassword, realm.Status.AdminPassword)
	assert.Equal(t, currentStatus.AdminUrl, realm.Status.AdminUrl)
	assert.Equal(t, currentStatus.AdminTokenUrl, realm.Status.AdminTokenUrl)
	assert.True(t, HasConditions(t, realm, []v1.Condition{doneProcessingCondition, readyCondition}))
}

func TestSetStatusReadyHandlesNilRealm(t *testing.T) {
	currentStatus := &identityv1.RealmStatus{
		IssuerUrl:     "https://issuer.example.com",
		AdminClientId: "admin-client-id",
		AdminUserName: "admin-username",
		AdminPassword: "admin-password",
		AdminUrl:      "https://admin.example.com",
		AdminTokenUrl: "https://admin.example.com/token",
	}
	var realm *identityv1.Realm

	assert.Panics(t, func() {
		SetStatusReady(currentStatus, realm)
	})
}

func HasConditions(t *testing.T, client *identityv1.Realm, expectedConditions []v1.Condition) bool {
	conditions := client.GetConditions()

	for _, expectedCondition := range expectedConditions {
		found := false
		for _, cond := range conditions {
			if cond.Type == expectedCondition.Type && cond.Status == expectedCondition.Status {
				found = true
			}
		}
		if !found {
			t.Logf("Condition not found: Type: '%v' and Message: '%v'", expectedCondition.Type, expectedCondition.Message)
			return false
		}
	}

	return true
}
