// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package filetype implements the FileType catalogue resource module for the
// projector.
package filetype

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// FileTypeKey is the composite identity key for FileType catalogue entities.
// File type identifiers are uniform across the environment, so the type string
// alone is sufficient.
type FileTypeKey struct {
	FileType string
}

// FileTypeData carries the transformed data for a FileType catalogue entity.
type FileTypeData struct {
	Meta          shared.Metadata
	StatusPhase   string // "READY", "PENDING", "ERROR", "UNKNOWN"
	StatusMessage string
	FileType      string
	Description   string
	Variant       *string
	Active        bool // Active indicates whether the FileType is currently in use by any FileExposure.
}
