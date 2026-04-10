// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package group implements the Group resource module for the projector.
// Group is a root entity (Level 0) with no FK dependencies.
package group

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// GroupKey is the identity key for Group entities, derived from metadata.name.
// Group uses the Strong delete strategy — the key is always derivable from
// the reconcile request without needing lastKnown state.
type GroupKey string

// GroupData carries the transformed data for a Group entity.
type GroupData struct {
	Meta        shared.Metadata
	Name        string
	DisplayName string
	Description string
}
