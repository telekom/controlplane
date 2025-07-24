// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
	"io"
)

var _ backend.FileUploader = &S3FileUploader{}

type S3FileUploader struct {
	config *S3Config
}

// NewS3FileUploader creates a new S3FileUploader with the given configuration
func NewS3FileUploader(config *S3Config) *S3FileUploader {
	return &S3FileUploader{
		config: config,
	}
}

// UploadFile uploads a file to S3 and returns the file ID
// The fileId should follow the convention <env>--<group>--<team>--<fileName>
// The file will be stored in S3 using a path format: <env>/<group>/<team>/<fileName>
// Metadata can include X-File-Content-Type and X-File-Checksum headers
func (s *S3FileUploader) UploadFile(ctx context.Context, fileId string, reader *io.Reader, metadata map[string]string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	if s.config == nil || s.config.Client == nil {
		log.Error(nil, "S3 client not initialized")
		return "", errors.New("S3 client not initialized")
	}

	// Extract bearer token from context and update client credentials
	token, err := ExtractBearerTokenFromContext(ctx)
	if err == nil {
		// Update token only if found in context
		if err := s.config.UpdateBearerToken(token); err != nil {
			log.Error(err, "Failed to update bearer token")
			// Continue with old token if update fails
		}
	} else {
		log.V(1).Info("No bearer token in context, using existing credentials")
	}

	if reader == nil || *reader == nil {
		log.Error(nil, "File reader is nil")
		return "", errors.New("file reader is nil")
	}

	// Convert fileId to S3 path format
	s3Path, err := identifier.ConvertFileIdToPath(fileId)
	if err != nil {
		log.Error(err, "Failed to convert fileId to S3 path")
		return "", errors.Wrap(err, "failed to convert fileId to S3 path")
	}

	log.V(1).Info("Uploading file", "fileId", fileId, "s3Path", s3Path, "bucket", s.config.BucketName)

	// Get content type from metadata or use default
	contentType := "application/octet-stream"
	if ctHeader, ok := metadata["X-File-Content-Type"]; ok && ctHeader != "" {
		contentType = ctHeader
		log.V(1).Info("Using content type from metadata", "contentType", contentType)
	}

	// Prepare minio UserMetadata from our metadata map
	userMetadata := make(map[string]string)

	// Add X-File-Content-Type to UserMetadata if present
	if value, ok := metadata["X-File-Content-Type"]; ok && value != "" {
		userMetadata["X-File-Content-Type"] = value
	}

	// Add X-File-Checksum to UserMetadata if present
	if value, ok := metadata["X-File-Checksum"]; ok && value != "" {
		userMetadata["X-File-Checksum"] = value
		log.V(1).Info("Added checksum to metadata", "checksum", value)
	}

	// Configure PutObjectOptions
	putOptions := minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: userMetadata,
		// TODO: CHECKSUM-SHA-256: Enable SHA-256 checksum calculation and verification
		//Checksum: minio.ChecksumSHA256,
	}

	// TODO: CHECKSUM-SHA-256: Enable SHA-256 checksum calculation and verification
	// If client provided a checksum, set it for server-side validation
	// S3 will automatically validate this against the uploaded content
	//if providedChecksum, ok := metadata["X-File-Checksum"]; ok && providedChecksum != "" {
	//	log.V(1).Info("Using client-provided checksum for server-side validation", "checksum", providedChecksum)
	//	// Set the full object checksum mode in UserMetadata
	//	// This is the proper way to specify the checksum mode in minio-go v7.0.95
	//	putOptions.UserMetadata["x-amz-checksum-mode"] = "FULL_OBJECT"
	//	// Add the client-provided checksum in the format expected by S3
	//	putOptions.UserMetadata["x-amz-checksum-sha256"] = providedChecksum
	//}

	// Upload file using the S3 path instead of fileId directly
	log.V(1).Info("Starting S3 PutObject operation")
	_, err = s.config.Client.PutObject(ctx, s.config.BucketName, s3Path, *reader, -1, putOptions)

	if err != nil {
		log.Error(err, "Failed to upload file to S3")
		return "", errors.Wrap(err, "failed to upload file")
	}
	log.V(1).Info("File uploaded successfully", "fileId", fileId, "s3Path", s3Path)

	// Get the object info to validate metadata
	log.V(1).Info("Retrieving object info for validation", "s3Path", s3Path)
	objInfo, err := s.config.Client.StatObject(ctx, s.config.BucketName, s3Path, minio.StatObjectOptions{})
	if err != nil {
		log.Error(err, "Failed to retrieve object info for validation")
		return "", errors.Wrap(err, "failed to retrieve object info for validation")
	}

	// Validate Content-Type if it was specified in the request
	if requestedContentType, ok := metadata["X-File-Content-Type"]; ok && requestedContentType != "" {
		if objInfo.ContentType != requestedContentType {
			log.Error(nil, "Content-Type mismatch",
				"expected", requestedContentType,
				"actual", objInfo.ContentType)
			return "", errors.Errorf("content type mismatch: expected %s, got %s",
				requestedContentType, objInfo.ContentType)
		}
		log.V(1).Info("Content-Type validation successful", "contentType", objInfo.ContentType)
	}

	// Validate Checksum if it was specified in the request
	if requestedChecksum, ok := metadata["X-File-Checksum"]; ok && requestedChecksum != "" {
		// Use the S3-generated checksum instead of the UserMetadata
		var storedChecksum string
		if objInfo.ETag != "" {
			// Use the S3-generated checksum if available
			storedChecksum = objInfo.ETag
			log.V(1).Info("Using S3-generated checksum for validation", "checksum", storedChecksum)
		} else {
			// Fall back to UserMetadata if ETag is not available
			storedChecksum = objInfo.UserMetadata["X-File-Checksum"]
			log.V(1).Info("Using UserMetadata checksum for validation", "userMetadataChecksum", storedChecksum)
		}

		// If checksum differs from what was requested, return an error
		if storedChecksum != requestedChecksum {
			checksumSource := "UserMetadata"
			if objInfo.ETag != "" {
				checksumSource = "S3 ETag"
			}
			log.Error(nil, "Checksum mismatch",
				"expected", requestedChecksum,
				"actual", storedChecksum,
				"checksumSource", checksumSource)
			return "", errors.Errorf("checksum mismatch: expected %s, got %s",
				requestedChecksum, storedChecksum)
		}
		log.V(1).Info("Checksum validation successful", "checksum", storedChecksum)
	}

	// Return the original fileId as that's the expected return value by the interface
	return fileId, nil
}
