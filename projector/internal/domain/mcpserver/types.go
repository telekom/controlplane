// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package mcpserver implements the McpServer catalogue resource module for
// the projector. McpServer is a Level 2 entity with a required FK dependency
// on Team.
package mcpserver

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// McpServerKey is the composite identity key for McpServer catalogue entities.
// McpServer base paths are unique per team, so both components are needed.
type McpServerKey struct {
	BasePath string
	TeamName string
}

// McpServerData carries the transformed data for an McpServer catalogue entity.
type McpServerData struct {
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
