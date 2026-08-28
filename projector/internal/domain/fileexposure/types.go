// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package fileexposure implements the FileExposure resource module for the
// projector. FileExposure is a Level 3 entity with required FK dependencies on
// Application (owner) and Zone.
package fileexposure

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// FileExposureKey is the composite identity key for FileExposure entities.
// File types are unique per application and applications are unique per team,
// so all three components are needed.
type FileExposureKey struct {
	FileType string
	AppName  string
	TeamName string
}

// FileExposureData carries the transformed data for a FileExposure entity.
type FileExposureData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	Provider       *string
	Visibility     string // "WORLD", "ZONE", "ENTERPRISE"
	Active         bool
	Zone           string
	SFTPPublicKeys []string
	ApprovalConfig model.ApprovalConfig
	AppName        string // resolved to owner Application FK
	TeamName       string // used to resolve owner Application FK
	TargetFileType string // optional FileType catalogue FK
}
