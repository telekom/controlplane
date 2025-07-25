// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/internal/middleware"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

// MinioWrapper provides common functionality for S3 operations
type MinioWrapper struct {
	config *S3Config
}

// NewMinioWrapper creates a new wrapper for Minio operations with the given configuration
func NewMinioWrapper(config *S3Config) *MinioWrapper {
	return &MinioWrapper{
		config: config,
	}
}

// UpdateCredentialsFromContext extracts and updates bearer token from the context if available
func (w *MinioWrapper) UpdateCredentialsFromContext(ctx context.Context) {
	log := logr.FromContextOrDiscard(ctx)

	// Extract bearer token from context and update client credentials
	token, err := middleware.ExtractBearerTokenFromContext(ctx)
	if err == nil {
		// Update token only if found in context
		if err := w.config.UpdateBearerToken(token); err != nil {
			log.Error(err, "Failed to update bearer token")
			// Continue with old token if update fails
		}
	} else {
		log.V(1).Info("No bearer token in context, using existing credentials")
	}
}

// GetObjectInfo retrieves S3 object metadata
func (w *MinioWrapper) GetObjectInfo(ctx context.Context, path string) (minio.ObjectInfo, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Getting object info for metadata", "s3Path", path)
	objInfo, err := w.config.Client.StatObject(ctx, w.config.BucketName, path, minio.StatObjectOptions{})
	if err != nil {
		log.Error(err, "Failed to get object info from S3")
		return minio.ObjectInfo{}, errors.Wrap(err, "failed to get object info from S3")
	}
	return objInfo, nil
}

// ExtractMetadata extracts relevant metadata from object info
func (w *MinioWrapper) ExtractMetadata(ctx context.Context, objInfo minio.ObjectInfo) map[string]string {
	log := logr.FromContextOrDiscard(ctx)
	metadata := make(map[string]string)

	// Add Content-Type to metadata
	if objInfo.ContentType != "" {
		metadata[backend.XFileContentType] = objInfo.ContentType
		log.V(1).Info("Added content type to response metadata", "contentType", objInfo.ContentType)
	}

	// Add Checksum to metadata
	// Prefer S3's Checksum over UserMetadata
	if objInfo.ETag != "" {
		metadata[backend.XFileChecksum] = objInfo.ETag
		log.V(1).Info("Added S3-generated checksum to response metadata", "checksum", objInfo.ETag)
	} else if checksum, ok := objInfo.UserMetadata[backend.XFileChecksum]; ok && checksum != "" {
		// Fall back to UserMetadata if ETag is not available
		metadata[backend.XFileChecksum] = checksum
		log.V(1).Info("Added UserMetadata checksum to response metadata", "checksum", checksum)
	}

	return metadata
}

// ValidateClient checks if the S3 client is properly initialized
func (w *MinioWrapper) ValidateClient(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	if w.config == nil || w.config.Client == nil {
		log.Error(nil, "S3 client not initialized")
		return errors.New("S3 client not initialized")
	}

	return nil
}

// validator is the internal reference to the object metadata validator
var validator *ObjectMetadataValidator

// ValidateObjectMetadata delegates to the ObjectMetadataValidator
// This method maintains backwards compatibility with existing code
func (w *MinioWrapper) ValidateObjectMetadata(ctx context.Context, path string, expectedContentType string, expectedChecksum string) error {
	// Create the validator if it doesn't exist yet
	if validator == nil {
		validator = NewObjectMetadataValidator(w)
	}

	// Delegate to the validator
	return validator.ValidateObjectMetadata(ctx, path, expectedContentType, expectedChecksum)
}
