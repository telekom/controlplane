// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package approvalrequest implements the ApprovalRequest resource module for
// the projector. ApprovalRequest is a Level 4 entity with a required
// FK dependency on ApiSubscription (M2O). It uses namespace+name as the unique
// conflict key and resolves the subscription FK via cache-based meta-key lookup.
package approvalrequest

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// ApprovalRequestKey is the composite identity key for ApprovalRequest
// entities. Namespace + Name form the unique index and are sufficient for both
// upsert conflict detection and delete identification.
// SubscriptionNamespace + SubscriptionName are carried for FK resolution
// during upsert and cache cleanup on delete.
type ApprovalRequestKey struct {
	Namespace             string
	Name                  string
	SubscriptionNamespace string
	SubscriptionName      string
}

// ApprovalRequestData carries the transformed data for an ApprovalRequest
// entity.
type ApprovalRequestData struct {
	Meta                 shared.Metadata
	StatusPhase          string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage        string
	State                string // "PENDING", "SEMIGRANTED", "GRANTED", "REJECTED"
	Action               string
	Strategy             string // "AUTO", "SIMPLE", "FOUR_EYES"
	Requester            model.RequesterInfo
	Decider              model.DeciderInfo
	Decisions            []model.Decision
	AvailableTransitions []model.AvailableTransition
	// ApiSubscription reference via spec.target (k8s namespace + name).
	SubscriptionNamespace string
	SubscriptionName      string
}
