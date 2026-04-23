// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package team implements the Team resource module for the projector.
// Team is a Level 1 entity with an optional FK dependency on Group and a
// nested entity pattern for Members.
package team

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// TeamKey is the identity key for Team entities, derived from metadata.name.
// The Team CR metadata.name is a composite "<group>--<team>" string.
// Team uses the Strong delete strategy — the key is always derivable from
// the reconcile request without needing lastKnown state.
type TeamKey string

// TeamData carries the transformed data for a Team entity.
type TeamData struct {
	Meta          shared.Metadata
	StatusPhase   string
	StatusMessage string
	Name          string
	Email         string
	Category      string // "CUSTOMER" or "INFRASTRUCTURE" (upper-cased from CR enum)
	GroupName     string
	Members       []MemberData
}

// MemberData carries the transformed data for a single team member.
type MemberData struct {
	Name  string
	Email string
}
