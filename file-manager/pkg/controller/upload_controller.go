// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
)

type UploadController interface {
	UploadFile(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error)
}

type uploadController struct {
	FileUploader backend.FileUploader
}

func NewUploadController(fu backend.FileUploader) UploadController {
	return &uploadController{FileUploader: fu}
}

// detectContentType detects the content type for a file based on its filename
// and adds appropriate metadata entries
func (u uploadController) detectContentType(ctx context.Context, fileName string, metadata map[string]string) (string, map[string]string) {
	log := logr.FromContextOrDiscard(ctx)

	// Make a copy of the metadata to avoid modifying the original
	if metadata == nil {
		metadata = make(map[string]string)
	} else {
		// Create a copy of the metadata
		metadataCopy := make(map[string]string, len(metadata))
		for k, v := range metadata {
			metadataCopy[k] = v
		}
		metadata = metadataCopy
	}

	// Detect content type from file extension
	detectedContentType := constants.DefaultContentType
	fileExt := filepath.Ext(fileName)
	if fileExt != "" {
		// Ensure the extension includes the dot and convert to lowercase
		if !strings.HasPrefix(fileExt, ".") {
			fileExt = "." + fileExt
		}
		tmpContentType := mime.TypeByExtension(fileExt)
		if tmpContentType != "" {
			detectedContentType = tmpContentType
			log.V(1).Info("Detected content type from file extension",
				"fileName", fileName,
				"extension", fileExt,
				"contentType", detectedContentType)
		}
	}

	// Get content type from metadata or use detected/default
	if ctHeader, ok := metadata[constants.XFileContentType]; ok && ctHeader != "" {
		// Check if the provided content type matches the detected one
		if detectedContentType != constants.DefaultContentType && ctHeader != detectedContentType {
			// Log a warning if content types don't match, but allow the upload to proceed
			log.V(1).Info("WARNING: Content type from metadata differs from detected type",
				"provided", ctHeader,
				"detected", detectedContentType,
				"fileName", fileName,
				"extension", filepath.Ext(fileName))

			// Store both content types in metadata for reference
			metadata[constants.XFileDetectedContentType] = detectedContentType
		}

		// Return the provided content type
		return ctHeader, metadata
	} else {
		log.V(1).Info("Using detected content type", "contentType", detectedContentType, "fileName", fileName)

		// Store the content type in metadata since it was auto-detected
		metadata[constants.XFileContentType] = detectedContentType
		metadata[constants.XFileContentTypeSource] = "auto-detected"

		// Return the detected content type
		return detectedContentType, metadata
	}
}

func (u uploadController) UploadFile(ctx context.Context, fileId string, reader *io.Reader, metadata map[string]string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Validate fileId format first
	if err := identifier.ValidateFileID(fileId); err != nil {
		return "", backend.ErrInvalidFileId(fileId)
	}

	// Validate reader input
	if reader == nil || *reader == nil {
		log.Error(nil, "File reader is nil")
		return "", backend.ErrUploadFailed(fileId, "file reader is nil")
	}

	// Parse the fileId to extract the filename for content type detection
	fileIdParts, err := identifier.ParseFileID(fileId)
	if err != nil {
		log.Error(err, "Failed to parse fileId for content type detection")
		return "", backend.ErrInvalidFileId(fileId)
	}

	// Detect content type and update metadata
	_, metadata = u.detectContentType(ctx, fileIdParts.FileName, metadata)

	// Use the fileUploader to upload the file with the fileId and processed metadata
	return u.FileUploader.UploadFile(ctx, fileId, *reader, metadata)
}
