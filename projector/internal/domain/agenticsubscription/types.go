// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package agenticsubscription implements the AgenticSubscription resource
// module for the projector. AgenticSubscription is a Level 3 entity with a
// required FK dependency on Application (owner) and an optional FK
// dependency on AgenticExposure (target). It uses pre-delete before upsert
// to handle target FK changes and maintains dual cache entries (primary +
// meta) for lookup by downstream Approval/ApprovalRequest modules.
package agenticsubscription

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// AgenticSubscriptionKey is the composite identity key for
// AgenticSubscription entities. It contains the fields needed for both the
// primary DB operation (BasePath + OwnerAppName + OwnerTeamName) and the meta
// cache cleanup on delete (Namespace + Name).
type AgenticSubscriptionKey struct {
	BasePath      string
	OwnerAppName  string
	OwnerTeamName string
	Namespace     string
	Name          string
}

// AgenticSubscriptionData carries the transformed data for an
// AgenticSubscription entity.
type AgenticSubscriptionData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	BasePath       string
	Security       *model.AgenticSubscriptionSecurity
	Traffic        *model.AgenticSubscriberTraffic
	OwnerAppName   string // resolved to owner Application FK (required)
	OwnerTeamName  string // used to resolve owner Application FK
	TargetBasePath string // used to resolve optional target AgenticExposure FK
}
