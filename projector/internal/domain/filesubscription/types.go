// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package filesubscription implements the FileSubscription resource module for
// the projector. FileSubscription is a Level 3 entity with required FK
// dependencies on Application (owner) and Zone, and optional FKs to
// FileExposure (target) and FileType (catalogue).
package filesubscription

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// FileSubscriptionKey is the composite identity key for FileSubscription
// entities and cache cleanup by metadata.
type FileSubscriptionKey struct {
	FileType      string
	OwnerAppName  string
	OwnerTeamName string
	Namespace     string
	Name          string
}

// FileSubscriptionData carries the transformed data for a FileSubscription entity.
type FileSubscriptionData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	ZoneName       string
	ZoneNamespace  *string
	SFTPPublicKeys []string
	OwnerAppName   string // resolved to owner Application FK (required)
	OwnerTeamName  string // used to resolve owner Application FK
	TargetFileType string // used to resolve optional target FileExposure FK
}
