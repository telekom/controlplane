// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/telekom/controlplane/common/pkg/condition"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
)

var (
	// Processing
	processingCondition = condition.NewProcessingCondition("ClientProcessing",
		"Processing client")
	processingNotReadyCondition = condition.NewNotReadyCondition("ClientNotReady",
		"Client not ready")

	// Blocked
	blockedCondition         = condition.NewBlockedCondition("Realm not found")
	blockedNotReadyCondition = condition.NewNotReadyCondition("RealmNotFound", "Realm not found")

	// Waiting
	waitingCondition = condition.NewProcessingCondition("ClientProcessing",
		"Waiting for Realm to be processed")
	waitingNotReadyCondition = condition.NewNotReadyCondition("ClientProcessing",
		"Waiting for Realm to be processed")

	// Ready
	doneProcessingCondition = condition.NewDoneProcessingCondition("Created Client")
	readyCondition          = condition.NewReadyCondition("Ready", "Client is ready")
)

func MapToClientStatus(realmStatus *identityv1.RealmStatus, clientStatus *identityv1.ClientStatus) {
	if clientStatus == nil {
		clientStatus = &identityv1.ClientStatus{}
	}

	clientStatus.IssuerUrl = realmStatus.IssuerUrl
}

func SetStatusProcessing(client *identityv1.Client) {
	client.SetCondition(processingCondition)
	client.SetCondition(processingNotReadyCondition)
}

func SetStatusBlocked(client *identityv1.Client) {
	client.SetCondition(blockedCondition)
	client.SetCondition(blockedNotReadyCondition)
}

func SetStatusWaiting(client *identityv1.Client) {
	client.SetCondition(waitingCondition)
	client.SetCondition(waitingNotReadyCondition)
}

func SetStatusReady(client *identityv1.Client) {
	client.SetCondition(doneProcessingCondition)
	client.SetCondition(readyCondition)
}
