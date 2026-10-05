// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"maps"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

type UploadController interface {
	UploadFile(ctx context.Context, fileId string, file io.Reader, metadata map[string]string) (string, error)
}

type uploadController struct {
	FileUploader backend.FileUploader
}

func NewUploadController(fu backend.FileUploader) UploadController {
	return &uploadController{FileUploader: fu}
}

// resolveContentType returns the content type from metadata, falling back to the default.
// File IDs are UUIDs without an extension, so the content type cannot be detected from the name.
func (u uploadController) resolveContentType(ctx context.Context, fileName string, metadata map[string]string) (string, map[string]string) {
	log := logr.FromContextOrDiscard(ctx)

	if ctHeader, ok := metadata[constants.XFileContentType]; ok && ctHeader != "" {
		return ctHeader, maps.Clone(metadata)
	}

	log.V(1).Info("No content type provided, using default", "contentType", constants.DefaultContentType, "fileName", fileName)

	newMetadata := make(map[string]string, len(metadata)+2)
	maps.Copy(newMetadata, metadata)

	newMetadata[constants.XFileContentType] = constants.DefaultContentType
	newMetadata[constants.XFileContentTypeSource] = "auto-detected"
	return constants.DefaultContentType, newMetadata
}

func (u uploadController) UploadFile(ctx context.Context, fileId string, reader io.Reader, metadata map[string]string) (string, error) {
	// TODO: backward-compatibility: uncomment it after all files were migrated to UUID
	// if err := identifier.ValidateFileID(fileId); err != nil {
	// 	return "", backend.ErrInvalidFileId(fileId)
	// }

	if reader == nil {
		return "", backend.ErrUploadFailed(fileId, "file reader is nil")
	}

	_, metadata = u.resolveContentType(ctx, fileId, metadata)

	return u.FileUploader.UploadFile(ctx, fileId, reader, metadata)
}
