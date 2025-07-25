// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"context"
	"io"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
)

var _ backend.FileUploader = &S3FileUploader{}

type S3FileUploader struct {
	config  *S3Config
	wrapper *MinioWrapper
}

// NewS3FileUploader creates a new S3FileUploader with the given configuration
func NewS3FileUploader(config *S3Config) *S3FileUploader {
	return &S3FileUploader{
		config:  config,
		wrapper: NewMinioWrapper(config),
	}
}

// prepareMetadata processes the input metadata and returns UserMetadata for S3 and the content type
func (s *S3FileUploader) prepareMetadata(ctx context.Context, metadata map[string]string) (map[string]string, string) {
	log := logr.FromContextOrDiscard(ctx)

	// Prepare minio UserMetadata from our metadata map
	userMetadata := make(map[string]string)
	// Copy all metadata fields to UserMetadata
	for k, v := range metadata {
		userMetadata[k] = v
	}

	// Get content type from metadata
	contentType := backend.DefaultContentType // fallback default
	if ctHeader, ok := metadata[backend.XFileContentType]; ok && ctHeader != "" {
		contentType = ctHeader
		log.V(1).Info("Using content type from metadata", "contentType", contentType)
	}

	// Add X-File-Checksum to UserMetadata if present and log it
	if value, ok := metadata[backend.XFileChecksum]; ok && value != "" {
		log.V(1).Info("Added checksum to metadata", "checksum", value)
	}

	return userMetadata, contentType
}

// uploadToS3 handles the actual upload operation to S3
func (s *S3FileUploader) uploadToS3(ctx context.Context, path string, reader io.Reader, contentType string, userMetadata map[string]string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Configure PutObjectOptions
	putOptions := minio.PutObjectOptions{
		ContentType:    contentType,
		UserMetadata:   userMetadata,
		SendContentMd5: true, // Enable MD5 checksum calculation and verification
	}

	// Upload file using the S3 path directly
	log.V(1).Info("Starting S3 PutObject operation")
	_, err := s.config.Client.PutObject(ctx, s.config.BucketName, path, reader, -1, putOptions)

	if err != nil {
		log.Error(err, "Failed to upload file to S3")
		return backend.ErrUploadFailed(path, err.Error())
	}

	return nil
}

// validateUploadedMetadata validates that the uploaded object metadata matches expectations
func (s *S3FileUploader) validateUploadedMetadata(ctx context.Context, path string, metadata map[string]string) error {
	// Extract metadata fields that need validation
	requestContentType := ""
	requestChecksum := ""

	// Get requested content type if provided
	if value, ok := metadata[backend.XFileContentType]; ok && value != "" {
		requestContentType = value
	}

	// Get requested checksum if provided
	if value, ok := metadata[backend.XFileChecksum]; ok && value != "" {
		requestChecksum = value
	}

	// Validate the metadata using the wrapper
	return s.wrapper.ValidateObjectMetadata(ctx, path, requestContentType, requestChecksum)
}

// UploadFile uploads a file to S3 and returns the file ID
// The fileId should follow the convention <env>--<group>--<team>--<fileName>
// Metadata already includes X-File-Content-Type and X-File-Checksum headers
// convertFileIdToPath converts a fileId to an S3 path and logs the result
func (s *S3FileUploader) convertFileIdToPath(ctx context.Context, fileId string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Convert fileId to S3 path format
	path, err := identifier.ConvertFileIdToPath(fileId)
	if err != nil {
		log.Error(err, "Failed to convert fileId to S3 path")
		return "", backend.ErrInvalidFileId(fileId)
	}

	log.V(1).Info("Using S3 path", "path", path)
	return path, nil
}

// initializeUpload validates the client and updates credentials
func (s *S3FileUploader) initializeUpload(ctx context.Context) error {
	// Validate client initialization
	if err := s.wrapper.ValidateClient(ctx); err != nil {
		return backend.ErrClientInitialization(err.Error())
	}

	// Update credentials using token from context if available
	s.wrapper.UpdateCredentialsFromContext(ctx)
	return nil
}

func (s *S3FileUploader) UploadFile(ctx context.Context, fileId string, reader io.Reader, metadata map[string]string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Initialize upload (client validation and credential updates)
	if err := s.initializeUpload(ctx); err != nil {
		return "", err
	}

	log.V(1).Info("Uploading file", "fileId", fileId, "bucket", s.config.BucketName)

	// Convert fileId to S3 path
	path, err := s.convertFileIdToPath(ctx, fileId)
	if err != nil {
		return "", err
	}

	// Prepare metadata for S3 upload
	userMetadata, contentType := s.prepareMetadata(ctx, metadata)

	// Upload file to S3
	err = s.uploadToS3(ctx, path, reader, contentType, userMetadata)
	if err != nil {
		return "", err
	}
	log.V(1).Info("File uploaded successfully", "fileId", fileId, "s3Path", path)

	// Validate metadata after upload
	if err := s.validateUploadedMetadata(ctx, path, metadata); err != nil {
		return "", err
	}

	// Return the original fileId
	return fileId, nil
}
