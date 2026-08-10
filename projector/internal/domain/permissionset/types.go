// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package permissionset implements the PermissionSet resource module for the
// projector operator. PermissionSet is a Level 3 entity with a required FK
// dependency on Application (which itself depends on Team + Zone). It is
// gated behind the FeaturePermission flag.
package permissionset

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// PermissionSetKey is the composite identity key for PermissionSet entities.
// PermissionSet is 1:1 with Application, so the owning application name and
// team name together uniquely identify it.
type PermissionSetKey struct {
	AppName  string
	TeamName string
}

// PermissionSetData carries the transformed data for a PermissionSet entity.
type PermissionSetData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	Permissions   []model.Permission
	AppName       string // resolved to owner Application FK
	TeamName      string // used to resolve owner Application FK
}
