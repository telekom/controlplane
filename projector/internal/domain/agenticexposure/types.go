// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package agenticexposure implements the AgenticExposure resource module for
// the projector. AgenticExposure is a Level 3 entity with a required FK
// dependency on Application (which itself depends on Team + Zone), and two
// mutually exclusive optional FK dependencies on McpServer/AgentCard,
// selected by the exposure's Variant (MCP/TELECONTEXTMCP → McpServer,
// AGENT → AgentCard).
package agenticexposure

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// AgenticExposureKey is the composite identity key for AgenticExposure
// entities. AgenticExposure base paths are unique per Application, and
// Applications are unique per Team, so all three components are needed.
type AgenticExposureKey struct {
	BasePath string
	AppName  string
	TeamName string
}

// AgenticExposureData carries the transformed data for an AgenticExposure entity.
type AgenticExposureData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	BasePath       string
	Visibility     string // "WORLD", "ZONE", "ENTERPRISE" (upper-cased)
	Variant        string // "MCP", "TELECONTEXTMCP", "AGENT"
	Active         bool
	Upstreams      []model.Upstream
	ApprovalConfig model.ApprovalConfig
	AppName        string                         // resolved to owner Application FK
	TeamName       string                         // used to resolve owner Application FK
	Security       *model.AgenticExposureSecurity // optional/nillable
	Traffic        *model.Traffic                 // optional/nillable
	Transformation *model.AgenticTransformation   // optional/nillable
}
