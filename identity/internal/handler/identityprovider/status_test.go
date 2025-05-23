// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identityprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

func TestMapToIdpStatusMapsCorrectly(t *testing.T) {
	idpSpec := &identityv1.IdentityProviderSpec{
		AdminUrl: "https://admin.example.com",
	}

	idpStatus := MapToIdpStatus(idpSpec)

	assert.Equal(t, "https://admin.example.com", idpStatus.AdminUrl)
	assert.Equal(t, keycloak.DetermineAdminTokenUrlFrom(idpSpec.AdminUrl, keycloak.MasterRealm), idpStatus.AdminTokenUrl)
	assert.Equal(t, keycloak.DetermineAdminConsoleUrlFrom(idpSpec.AdminUrl, keycloak.MasterRealm), idpStatus.AdminConsoleUrl)
}

func TestSetStatusReadySetsIdpStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.IdentityProviderStatus{
		AdminUrl: "https://admin.example.com",
	}
	idp := &identityv1.IdentityProvider{}

	SetStatusReady(currentStatus, idp)

	assert.Equal(t, currentStatus.AdminUrl, idp.Status.AdminUrl)
	assert.Equal(t, currentStatus.AdminTokenUrl, idp.Status.AdminTokenUrl)
	assert.Equal(t, currentStatus.AdminConsoleUrl, idp.Status.AdminConsoleUrl)
	assert.True(t, HasConditions(t, idp, []v1.Condition{doneProcessingCondition, readyCondition}))
}

func TestSetStatusReadyHandlesNilIdp(t *testing.T) {
	currentStatus := &identityv1.IdentityProviderStatus{
		AdminUrl: "https://admin.example.com",
	}
	var idp *identityv1.IdentityProvider

	assert.Panics(t, func() {
		SetStatusReady(currentStatus, idp)
	})
}

func HasConditions(t *testing.T, client *identityv1.IdentityProvider, expectedConditions []v1.Condition) bool {
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
