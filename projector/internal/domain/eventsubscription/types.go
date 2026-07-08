// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package eventsubscription implements the EventSubscription resource module
// for the projector. EventSubscription is a Level 3 entity with a required
// FK dependency on Application (owner) and an optional FK dependency on
// EventExposure (target).
package eventsubscription

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// EventSubscriptionKey is the composite identity key for EventSubscription
// entities. It contains the fields needed for both the primary DB operation
// (EventType + OwnerAppName + OwnerTeamName) and the meta cache cleanup on
// delete (Namespace + Name).
type EventSubscriptionKey struct {
	EventType     string
	OwnerAppName  string
	OwnerTeamName string
	Namespace     string
	Name          string
}

// EventSubscriptionData carries the transformed data for an EventSubscription entity.
type EventSubscriptionData struct {
	Meta            shared.Metadata
	StatusPhase     string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage   string
	EventType       string
	DeliveryType    string  // "CALLBACK", "SERVER_SENT_EVENT"
	CallbackURL     *string // set when delivery type is Callback
	Delivery        *model.EventDelivery
	Trigger         *model.EventTrigger
	Scopes          []string
	OwnerAppName    string // resolved to owner Application FK (required)
	OwnerTeamName   string // used to resolve owner Application FK
	TargetEventType string // used to resolve optional target EventExposure FK
}
