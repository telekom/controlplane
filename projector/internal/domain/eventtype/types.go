// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package eventtype implements the EventType catalogue resource module for the
// projector. EventType is a Level 2 entity with a required FK dependency on Team.
package eventtype

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// EventTypeKey is the composite identity key for EventType catalogue entities.
// Event type identifiers are unique per team, so both components are needed.
type EventTypeKey struct {
	EventType string
	TeamName  string
}

// EventTypeData carries the transformed data for an EventType catalogue entity.
type EventTypeData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	EventType     string
	Version       string
	Description   string
	Specification string // file-manager file ID (optional)
	Active        bool   // cluster-wide active singleton flag
	TeamName      string // resolved to owner Team FK
}
