// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package apiexposure implements the ApiExposure resource module for the projector
// operator. ApiExposure is a Level 3 entity with a required FK dependency
// on Application (which itself depends on Team + Zone).
package apiexposure

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
)

// APIExposureKey is the composite identity key for ApiExposure entities.
// ApiExposure base paths are unique per Application, and Applications are
// unique per Team, so all three components are needed.
type APIExposureKey struct {
	BasePath string
	AppName  string
	TeamName string
}

// APIExposureData carries the transformed data for an ApiExposure entity.
type APIExposureData struct {
	Meta           shared.Metadata
	StatusPhase    string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage  string
	BasePath       string
	Visibility     string // "WORLD", "ZONE", "ENTERPRISE" (upper-cased)
	Active         bool
	Features       []string
	Upstreams      []model.Upstream
	ApprovalConfig model.ApprovalConfig
	APIVersion     *string // optional/nillable — always nil from CR
	AppName        string  // resolved to owner Application FK
	TeamName       string  // used to resolve owner Application FK
}
