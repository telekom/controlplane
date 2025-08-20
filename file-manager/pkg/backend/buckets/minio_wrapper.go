// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

// MinioWrapper provides common functionality for bucket operations
type MinioWrapper struct {
	config *BucketConfig
}

// NewMinioWrapper creates a new wrapper for Minio operations with the given configuration
func NewMinioWrapper(config *BucketConfig) *MinioWrapper {
	return &MinioWrapper{
		config: config,
	}
}

// UpdateCredentialsFromContext now refreshes credentials from the token source, ignoring the request context.
func (w *MinioWrapper) UpdateCredentialsFromContext(ctx context.Context) {
	log := logr.FromContextOrDiscard(ctx)
	if err := w.config.RefreshCredentialsOrDiscard(); err != nil {
		log.Error(err, "Failed to refresh credentials from token source")
	}
}

// GetObjectInfo retrieves object metadata
func (w *MinioWrapper) GetObjectInfo(ctx context.Context, path string) (minio.ObjectInfo, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Getting object info for metadata", "path", path)
	objInfo, err := w.config.Client.StatObject(ctx, w.config.BucketName, path, minio.StatObjectOptions{})
	if err != nil {
		log.Error(err, "Failed to get object info from bucket")
		return minio.ObjectInfo{}, backend.ErrFileNotFound(path)
	}
	return objInfo, nil
}

// ExtractMetadata extracts relevant metadata from object info
func (w *MinioWrapper) ExtractMetadata(ctx context.Context, objInfo minio.ObjectInfo) map[string]string {
	log := logr.FromContextOrDiscard(ctx)
	metadata := make(map[string]string)

	log.V(1).Info("Extracting metadata from object info", "objectInfo", objInfo)

	// Add Content-Type to metadata
	if objInfo.ContentType != "" {
		metadata[constants.XFileContentType] = objInfo.ContentType
		log.V(1).Info("Added content type to response metadata", "contentType", objInfo.ContentType)
	}

	checksum := objInfo.Metadata.Get("X-Amz-Meta-X-File-Checksum")
	if checksum != "" {
		metadata[constants.XFileChecksum] = objInfo.Metadata.Get("X-Amz-Meta-X-File-Checksum")
		log.V(1).Info("Added checksum to response metadata", "checksum", checksum)
	}

	return metadata
}

// ValidateClient checks if the client is properly initialized
func (w *MinioWrapper) ValidateClient(ctx context.Context) error {
	if w.config == nil || w.config.Client == nil {
		return backend.ErrClientInitialization("client not initialized")
	}

	return nil
}

// validator is the internal reference to the object metadata validator
var validator *ObjectMetadataValidator

// ValidateObjectMetadata delegates to the ObjectMetadataValidator
// This method maintains backwards compatibility with existing code
func (w *MinioWrapper) ValidateObjectMetadata(ctx context.Context, path string, expectedContentType string, expectedChecksum string, uploadedCRC64 string) error {
	// Create the validator if it doesn't exist yet
	if validator == nil {
		validator = NewObjectMetadataValidator(w)
	}

	// Delegate to the validator
	return validator.ValidateObjectMetadata(ctx, path, expectedContentType, expectedChecksum, uploadedCRC64)
}
