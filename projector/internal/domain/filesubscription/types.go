// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package filesubscription implements the FileSubscription resource module for
// the projector. FileSubscription is a Level 3 entity with required FK
// dependencies on Application (owner) and Zone, and optional FKs to
// FileExposure (target) and FileType (catalogue).
package filesubscription

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// FileSubscriptionKey is the composite identity key for FileSubscription
// entities. It contains the fields needed for both the primary DB operation
// (FileType + OwnerAppName + OwnerTeamName) and the meta cache cleanup on
// delete (Namespace + Name).
type FileSubscriptionKey struct {
	FileType      string
	OwnerAppName  string
	OwnerTeamName string
	Namespace     string
	Name          string
}

// FileSubscriptionData carries the transformed data for a FileSubscription entity.
type FileSubscriptionData struct {
	Meta               shared.Metadata
	StatusPhase        string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage      string
	Zone               string
	FileSFTP           *model.FileSFTP
	OwnerAppName       string // resolved to owner Application FK (required)
	OwnerTeamName      string // used to resolve owner Application FK
	TargetFileType     string // used to resolve optional target FileExposure FK
	ServiceURL         string // represents the internal SFTP service endpoint for users.
	ServiceExternalURL string // represents the externally reachable SFTP service endpoint for users.
}
