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
// Namespace and Name identify the Kubernetes resource. Delete uses them to
// find the entity when BasePath is empty.
type McpServerKey struct {
	BasePath  string
	TeamName  string
	Namespace string
	Name      string
}

// McpServerData carries the transformed data for an McpServer catalogue entity.
type McpServerData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	BasePath      string
	Version       string
	Name          string // resource name (metadata.name)
	DisplayName   string // human-readable name (spec.name)
	Description   string
	Category      string
	Oauth2Scopes  []string
	Specification string // file-manager file ID (optional)
	Hash          string // SHA-256 hash of the specification content (optional)
	Active        bool   // cluster-wide active singleton flag
	TeamName      string // resolved to owner Team FK
}
