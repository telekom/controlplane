// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

func TestMapToClientStatusMapsCorrectly(t *testing.T) {
	realmStatus := &identityv1.RealmStatus{
		IssuerUrl: "https://issuer.example.com",
	}

	clientStatus := MapToClientStatus(realmStatus)

	assert.Equal(t, "https://issuer.example.com", clientStatus.IssuerUrl)
}

func TestSetStatusProcessingSetsClientStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.ClientStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	client := &identityv1.Client{}

	SetStatusProcessing(currentStatus, client)

	assert.Equal(t, currentStatus.IssuerUrl, client.Status.IssuerUrl)
	assert.True(t, HasConditions(t, client, []v1.Condition{processingCondition, processingNotReadyCondition}))
}

func TestSetStatusBlockedSetsClientStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.ClientStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	client := &identityv1.Client{}

	SetStatusBlocked(currentStatus, client)

	assert.Equal(t, currentStatus.IssuerUrl, client.Status.IssuerUrl)
	assert.True(t, HasConditions(t, client, []v1.Condition{blockedCondition, blockedNotReadyCondition}))
}

func TestSetStatusWaitingSetsClientStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.ClientStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	client := &identityv1.Client{}

	SetStatusWaiting(currentStatus, client)

	assert.Equal(t, currentStatus.IssuerUrl, client.Status.IssuerUrl)
	assert.True(t, HasConditions(t, client, []v1.Condition{waitingCondition, waitingNotReadyCondition}))
}

func TestSetStatusReadySetsClientStatusCorrectly(t *testing.T) {
	currentStatus := &identityv1.ClientStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	client := &identityv1.Client{}

	SetStatusReady(currentStatus, client)

	assert.Equal(t, currentStatus.IssuerUrl, client.Status.IssuerUrl)
	assert.True(t, HasConditions(t, client, []v1.Condition{doneProcessingCondition, readyCondition}))
}

func TestSetStatusReadyHandlesNilClient(t *testing.T) {
	currentStatus := &identityv1.ClientStatus{
		IssuerUrl: "https://issuer.example.com",
	}
	var client *identityv1.Client

	assert.Panics(t, func() {
		SetStatusReady(currentStatus, client)
	})
}

func HasConditions(t *testing.T, client *identityv1.Client, expectedConditions []v1.Condition) bool {
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
