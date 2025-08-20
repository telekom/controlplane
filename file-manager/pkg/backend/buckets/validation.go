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

// BucketClientValidator defines the interface for client validation
type BucketClientValidator interface {
	ValidateClient(ctx context.Context) error
	GetObjectInfo(ctx context.Context, path string) (minio.ObjectInfo, error)
}

// ObjectMetadataValidator handles validation of object metadata
type ObjectMetadataValidator struct {
	// Wrapper reference for using existing bucket operations
	wrapper BucketClientValidator
}

// NewObjectMetadataValidator creates a new validator with the provided wrapper
func NewObjectMetadataValidator(wrapper BucketClientValidator) *ObjectMetadataValidator {
	return &ObjectMetadataValidator{
		wrapper: wrapper,
	}
}

// ValidateObjectMetadata validates that the uploaded object metadata matches expectations
func (v *ObjectMetadataValidator) ValidateObjectMetadata(ctx context.Context, path string, expectedContentType string, expectedChecksum string, uploadedCRC64 string) error {
	log := logr.FromContextOrDiscard(ctx)

	// First validate the client
	if err := v.wrapper.ValidateClient(ctx); err != nil {
		return backend.ErrClientInitialization(err.Error())
	}

	// Get the object info to validate metadata
	log.V(1).Info("Retrieving object info for validation", "path", path)
	objInfo, err := v.wrapper.GetObjectInfo(ctx, path)
	if err != nil {
		log.Error(err, "Failed to retrieve object info for validation")
		return backend.ErrDownloadFailed(path, "failed to retrieve object info: "+err.Error())
	}

	// Validate Content-Type if provided
	if err := v.validateContentType(ctx, objInfo.ContentType, expectedContentType); err != nil {
		return err
	}

	// Validate Checksum if provided
	if err := v.validateChecksum(ctx, objInfo, expectedChecksum, uploadedCRC64); err != nil {
		return err
	}

	return nil
}

// validateContentType checks if the actual content type matches the expected content type
func (v *ObjectMetadataValidator) validateContentType(ctx context.Context, actualContentType, expectedContentType string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Skip validation if no content type was expected
	if expectedContentType == "" {
		return nil
	}

	if actualContentType != expectedContentType {
		log.Error(nil, "Content-Type mismatch",
			"expected", expectedContentType,
			"actual", actualContentType)
		return backend.ErrInvalidContentType("", expectedContentType, actualContentType)
	}

	log.V(1).Info("Content-Type validation successful", "contentType", actualContentType)
	return nil
}

// validateChecksum checks if the stored checksum matches the expected checksum
func (v *ObjectMetadataValidator) validateChecksum(ctx context.Context, objInfo interface{}, expectedChecksum string, uploadedCRC64 string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Skip validation if no checksum was expected
	if expectedChecksum == "" {
		return nil
	}

	// Access object info fields directly since we're using minio.ObjectInfo
	objInfoTyped, ok := objInfo.(minio.ObjectInfo)
	if !ok {
		return backend.ErrClientInitialization("invalid object info type for checksum validation")
	}

	// Use uploadedCRC64 if provided, otherwise fall back to object info
	var storedChecksum string
	if uploadedCRC64 != "" {
		storedChecksum = uploadedCRC64
		log.V(1).Info("Using uploaded CRC64NVME checksum for validation", "checksum", storedChecksum)
	} else if objInfoTyped.ChecksumCRC64NVME != "" {
		storedChecksum = objInfoTyped.ChecksumCRC64NVME
		log.V(1).Info("Using CRC64NVME checksum from object info for validation", "checksum", storedChecksum)
	} else if objInfoTyped.ETag != "" {
		storedChecksum = objInfoTyped.ETag
		log.V(1).Info("Using generated checksum (ETag) for validation", "checksum", storedChecksum)
	} else {
		// Fall back to UserMetadata if neither CRC64 nor ETag is available
		storedChecksum = objInfoTyped.UserMetadata[constants.XFileChecksum]
		log.V(1).Info("Using UserMetadata checksum for validation", "userMetadataChecksum", storedChecksum)
	}

	// If checksum differs from what was expected, return an error
	if storedChecksum != expectedChecksum {
		checksumSource := "UserMetadata"
		if uploadedCRC64 != "" {
			checksumSource = "UploadedCRC64NVME"
		} else if objInfoTyped.ChecksumCRC64NVME != "" {
			checksumSource = "CRC64NVME"
		} else if objInfoTyped.ETag != "" {
			checksumSource = "S3 ETag"
		}
		log.Error(nil, "Checksum mismatch",
			"expected", expectedChecksum,
			"actual", storedChecksum,
			"checksumSource", checksumSource)
		return backend.ErrInvalidChecksum(objInfoTyped.Key, expectedChecksum, storedChecksum)
	}

	log.V(1).Info("Checksum validation successful", "checksum", storedChecksum)
	return nil
}
