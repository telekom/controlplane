// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package eventexposure implements the EventExposure resource module for the
// projector. EventExposure is a Level 3 entity with a required FK dependency
// on Application (which itself depends on Team + Zone).
package eventexposure

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// EventExposureKey is the composite identity key for EventExposure entities.
// Event types are unique per Application, and Applications are unique per
// Team, so all three components are needed.
type EventExposureKey struct {
	EventType string
	AppName   string
	TeamName  string
}

// EventExposureData carries the transformed data for an EventExposure entity.
type EventExposureData struct {
	Meta               shared.Metadata
	StatusPhase        string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage      string
	EventType          string
	Visibility         string // "WORLD", "ZONE", "ENTERPRISE" (upper-cased)
	Active             bool
	ApprovalConfig     model.ApprovalConfig
	Scopes             []model.EventScope
	GatewayProviderUrl string
	AppName            string // resolved to owner Application FK
	TeamName           string // used to resolve owner Application FK
}
