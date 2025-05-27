// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"github.com/telekom/controlplane/common/pkg/condition"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

var (
	// Processing
	processingCondition = condition.NewProcessingCondition("RealmProcessing",
		"Processing realm")
	processingNotReadyCondition = condition.NewNotReadyCondition("RealmNotReady",
		"Realm not ready")

	// Blocked
	blockedCondition         = condition.NewBlockedCondition("IdentityProvider not found")
	blockedNotReadyCondition = condition.NewNotReadyCondition("IdentityProviderNotFound",
		"IdentityProvider not found")

	// Waiting
	waitingCondition = condition.NewProcessingCondition("RealmProcessing",
		"Waiting for IdentityProvider to be processed")
	waitingNotReadyCondition = condition.NewNotReadyCondition("RealmProcessing",
		"Waiting for IdentityProvider to be processed")

	// Ready
	doneProcessingCondition = condition.NewDoneProcessingCondition("Created Realm")
	readyCondition          = condition.NewReadyCondition("Ready", "Realm is ready")
)

func MapToRealmStatus(identityProvider *identityv1.IdentityProvider, realmName string) identityv1.RealmStatus {
	return identityv1.RealmStatus{
		IssuerUrl:     keycloak.DetermineIssuerUrlFrom(identityProvider.Spec.AdminUrl, realmName),
		AdminClientId: identityProvider.Spec.AdminClientId,
		AdminUserName: identityProvider.Spec.AdminUserName,
		AdminPassword: identityProvider.Spec.AdminPassword,
		AdminUrl:      identityProvider.Status.AdminUrl,
		AdminTokenUrl: identityProvider.Status.AdminTokenUrl,
	}
}

func SetStatusProcessing(currentStatus *identityv1.RealmStatus, client *identityv1.Realm) {
	client.Status = *currentStatus
	client.SetCondition(processingCondition)
	client.SetCondition(processingNotReadyCondition)
}

func SetStatusBlocked(currentStatus *identityv1.RealmStatus, realm *identityv1.Realm) {
	realm.Status = *currentStatus
	realm.SetCondition(blockedCondition)
	realm.SetCondition(blockedNotReadyCondition)
}

func SetStatusWaiting(currentStatus *identityv1.RealmStatus, realm *identityv1.Realm) {
	realm.Status = *currentStatus
	realm.SetCondition(waitingCondition)
	realm.SetCondition(waitingNotReadyCondition)
}

func SetStatusReady(currentStatus *identityv1.RealmStatus, realm *identityv1.Realm) {
	realm.Status = *currentStatus
	realm.SetCondition(doneProcessingCondition)
	realm.SetCondition(readyCondition)
}
