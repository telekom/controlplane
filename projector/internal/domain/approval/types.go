// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package approval implements the Approval resource module for the projector
// operator. Approval is a Level 4 entity with a required FK dependency
// on ApiSubscription. It uses namespace+name as the unique conflict key and
// resolves the subscription FK via cache-based meta-key lookup.
package approval

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// ApprovalKey is the composite identity key for Approval entities.
// Namespace + Name form the unique index and are sufficient for both
// upsert conflict detection and delete identification.
// SubscriptionNamespace + SubscriptionName are carried for FK resolution
// during upsert and cache cleanup on delete.
type ApprovalKey struct {
	Namespace             string
	Name                  string
	SubscriptionNamespace string
	SubscriptionName      string
}

// ApprovalData carries the transformed data for an Approval entity.
type ApprovalData struct {
	Meta                 shared.Metadata
	StatusPhase          string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage        string
	State                string // "PENDING", "SEMIGRANTED", "GRANTED", "REJECTED", "SUSPENDED", "EXPIRED"
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
