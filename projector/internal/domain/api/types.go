// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package api implements the Api catalogue resource module for the projector.
// Api is a Level 2 entity with a required FK dependency on Team.
package api

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// APIKey is the composite identity key for Api catalogue entities.
// Api base paths are unique per team, so both components are needed.
type APIKey struct {
	BasePath string
	TeamName string
}

// APIData carries the transformed data for an Api catalogue entity.
type APIData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	BasePath      string
	Version       string
	Category      string
	OAuth2Scopes  []string
	XVendor       bool
	Specification string // file-manager file ID (optional)
	Active        bool   // cluster-wide active singleton flag
	TeamName      string // resolved to owner Team FK
}
