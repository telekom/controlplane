// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package agentcard implements the AgentCard catalogue resource module for
// the projector. AgentCard is a Level 2 entity with a required FK dependency
// on Team.
package agentcard

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// AgentCardKey is the composite identity key for AgentCard catalogue entities.
// AgentCard base paths are unique per team, so both components are needed.
type AgentCardKey struct {
	BasePath string
	TeamName string
}

// AgentCardData carries the transformed data for an AgentCard catalogue entity.
type AgentCardData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	BasePath      string
	Version       string
	Name          string
	Description   string
	Category      string
	Oauth2Scopes  []string
	Specification string // file-manager file ID (optional)
	Active        bool   // cluster-wide active singleton flag
	TeamName      string // resolved to owner Team FK
}
