// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"bytes"
	"context"
	"io"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/telekom/controlplane/file-manager/api/constants"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
)

var _ backend.FileUploader = &BucketFileUploader{}

type BucketFileUploader struct {
	config  *BucketConfig
	wrapper *MinioWrapper
}

// NewBucketFileUploader creates a new BucketFileUploader with the given configuration
func NewBucketFileUploader(config *BucketConfig) *BucketFileUploader {
	return &BucketFileUploader{
		config:  config,
		wrapper: NewMinioWrapper(config),
	}
}

// prepareMetadata processes the input metadata and returns UserMetadata for bucket and the content type
func (s *BucketFileUploader) prepareMetadata(ctx context.Context, metadata map[string]string) (map[string]string, string) {
	log := logr.FromContextOrDiscard(ctx)

	// Prepare minio UserMetadata from our metadata map
	userMetadata := make(map[string]string)
	// Copy all metadata fields to UserMetadata
	for k, v := range metadata {
		userMetadata[k] = v
	}

	// Get content type from metadata
	contentType := constants.DefaultContentType // fallback default
	if ctHeader, ok := metadata[constants.XFileContentType]; ok && ctHeader != "" {
		contentType = ctHeader
		log.V(1).Info("Using content type from metadata", "contentType", contentType)
	}

	// Add X-File-Checksum to UserMetadata if present and log it
	if value, ok := metadata[constants.XFileChecksum]; ok && value != "" {
		log.V(1).Info("Added checksum to metadata", "checksum", value)
	}

	return userMetadata, contentType
}

// uploadToBucket handles the actual upload operation to the bucket
func (s *BucketFileUploader) uploadToBucket(ctx context.Context, path string, reader io.Reader, contentType string, userMetadata map[string]string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Configure PutObjectOptions
	putOptions := minio.PutObjectOptions{
		ContentType:    contentType,
		UserMetadata:   userMetadata,
		SendContentMd5: false,                   // Disable MD5 checksum calculation as we're using CRC64NVME
		Checksum:       minio.ChecksumCRC64NVME, // Use CRC64NVME checksum algorithm
	}

	// Upload file using the path directly
	log.V(1).Info("Starting bucket PutObject operation", "path", path, "putOptions", putOptions)

	copyReader, contentSize, err := copyAndMeasure(reader)
	if err != nil {
		return "", backend.ErrUploadFailed(path, "Failed to measure content size :"+err.Error())
	}

	uploadInfo, err := s.config.Client.PutObject(ctx, s.config.BucketName, path, copyReader, contentSize, putOptions)
	log.V(1).Info("Finished bucket PutObject operation", "uploadInfo", uploadInfo)

	if err != nil {
		log.Error(err, "Failed to upload file to bucket")
		return "", backend.ErrUploadFailed(path, err.Error())
	}

	return uploadInfo.ChecksumCRC64NVME, nil
}

// validateUploadedMetadata validates that the uploaded object metadata matches expectations
func (s *BucketFileUploader) validateUploadedMetadata(ctx context.Context, path string, metadata map[string]string, uploadedCRC64 string) error {
	// Extract metadata fields that need validation
	requestContentType := ""
	requestChecksum := ""

	// Get requested content type if provided
	if value, ok := metadata[constants.XFileContentType]; ok && value != "" {
		requestContentType = value
	}

	// Get requested checksum if provided
	if value, ok := metadata[constants.XFileChecksum]; ok && value != "" {
		requestChecksum = value
	}

	// Validate the metadata using the wrapper
	return s.wrapper.ValidateObjectMetadata(ctx, path, requestContentType, requestChecksum, uploadedCRC64)
}

// convertFileIdToPath converts a fileId to a path and logs the result
func (s *BucketFileUploader) convertFileIdToPath(ctx context.Context, fileId string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Convert fileId to path format
	path, err := identifier.ConvertFileIdToPath(fileId)
	if err != nil {
		log.Error(err, "Failed to convert fileId to path")
		return "", backend.ErrInvalidFileId(fileId)
	}

	log.V(1).Info("Using path", "path", path)
	return path, nil
}

// initializeUpload validates the client and updates credentials
func (s *BucketFileUploader) initializeUpload(ctx context.Context) error {
	// Validate client initialization
	if err := s.wrapper.ValidateClient(ctx); err != nil {
		return backend.ErrClientInitialization(err.Error())
	}

	// Update credentials using token from context if available
	s.wrapper.UpdateCredentialsFromContext(ctx)
	return nil
}

// UploadFile uploads a file to bucket and returns the file ID
// The fileId should follow the convention <env>--<group>--<team>--<fileName>
// Metadata already includes X-File-Content-Type and X-File-Checksum headers
func (s *BucketFileUploader) UploadFile(ctx context.Context, fileId string, reader io.Reader, metadata map[string]string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Initialize upload (client validation and credential updates)
	if err := s.initializeUpload(ctx); err != nil {
		return "", err
	}

	log.V(1).Info("Uploading file", "fileId", fileId, "bucket", s.config.BucketName)

	// Convert fileId to path
	path, err := s.convertFileIdToPath(ctx, fileId)
	if err != nil {
		return "", err
	}

	// Prepare metadata for upload
	userMetadata, contentType := s.prepareMetadata(ctx, metadata)

	// Upload file to bucket and get CRC64
	uploadedCRC64, err := s.uploadToBucket(ctx, path, reader, contentType, userMetadata)
	if err != nil {
		return "", err
	}
	log.V(1).Info("File uploaded successfully", "fileId", fileId, "path", path)

	// Validate metadata after upload, passing CRC64
	if err := s.validateUploadedMetadata(ctx, path, metadata, uploadedCRC64); err != nil {
		return "", err
	}

	// Return the original fileId
	return fileId, nil
}

func copyAndMeasure(r io.Reader) (io.Reader, int64, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, r)
	if err != nil {
		return nil, 0, err
	}
	return bytes.NewReader(buf.Bytes()), n, nil
}
