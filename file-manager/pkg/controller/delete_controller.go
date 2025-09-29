// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
)

type DeleteController interface {
	DeleteFile(ctx context.Context, fileId string) error
}

type deleteController struct {
	Deleter backend.FileDeleter
}

// NewDeleteController creates a DeleteController. If deleter is nil, a no-op
// controller is returned that always reports NotFound. This allows wiring to
// be added later without breaking builds.
func NewDeleteController(deleter backend.FileDeleter) DeleteController {
	return &deleteController{Deleter: deleter}
}

func (d deleteController) DeleteFile(ctx context.Context, fileId string) error {
	// Validate fileId format first
	if err := identifier.ValidateFileID(fileId); err != nil {
		return backend.ErrInvalidFileId(fileId)
	}

	// Convert fileId to path format
	path, err := identifier.ConvertFileIdToPath(fileId)
	if err != nil {
		return backend.ErrInvalidFileId(fileId)
	}

	return d.Deleter.DeleteFile(ctx, path)
}
