// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package apisubscription implements the ApiSubscription resource module for
// the projector. ApiSubscription is a Level 3 entity with a required
// FK dependency on Application (owner) and an optional FK dependency on
// ApiExposure (target). It uses pre-delete before upsert to handle target FK
// changes and maintains dual cache entries (primary + meta) for lookup by
// downstream Approval/ApprovalRequest modules.
package apisubscription

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// APISubscriptionKey is the composite identity key for ApiSubscription
// entities. It contains the fields needed for both the primary DB operation
// (BasePath + OwnerAppName + OwnerTeamName) and the meta cache cleanup on
// delete (Namespace + Name).
type APISubscriptionKey struct {
	BasePath      string
	OwnerAppName  string
	OwnerTeamName string
	Namespace     string
	Name          string
}

// APISubscriptionData carries the transformed data for an ApiSubscription entity.
type APISubscriptionData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	BasePath       string
	M2MAuthMethod  string // "NONE", "OAUTH2_CLIENT", "BASIC_AUTH", "SCOPES_ONLY"
	ApprovedScopes []string
	OwnerAppName   string // resolved to owner Application FK (required)
	OwnerTeamName  string // used to resolve owner Application FK
	TargetBasePath string // used to resolve optional target ApiExposure FK
	TargetAppName  string // always "" from CR (not known to subscriber)
	TargetTeamName string // always "" from CR (not known to subscriber)
}
