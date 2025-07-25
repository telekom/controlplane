// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"bytes"
	"context"
	"io"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

var _ backend.FileDownloader = &S3FileDownloader{}

// S3FileDownloader implements backend.FileDownloader for S3

type S3FileDownloader struct {
	config  *S3Config
	wrapper *MinioWrapper
}

// NewS3FileDownloader creates a new S3FileDownloader with the given configuration
func NewS3FileDownloader(config *S3Config) *S3FileDownloader {
	return &S3FileDownloader{
		config:  config,
		wrapper: NewMinioWrapper(config),
	}
}

// downloadObject downloads the S3 object and returns it as a buffer
func (s *S3FileDownloader) downloadObject(ctx context.Context, path string) (*bytes.Buffer, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Starting S3 GetObject operation")
	object, err := s.config.Client.GetObject(ctx, s.config.BucketName, path, minio.GetObjectOptions{})
	if err != nil {
		log.Error(err, "Failed to get file from S3")
		return nil, errors.Wrap(err, "failed to get file from S3")
	}
	defer object.Close() //nolint:errcheck

	// Create a buffer to store the downloaded content
	buf := new(bytes.Buffer)

	// Copy object data to buffer
	_, err = io.Copy(buf, object)
	if err != nil {
		log.Error(err, "Failed to read file data")
		return nil, errors.Wrap(err, "failed to read file data")
	}

	return buf, nil
}

// DownloadFile downloads a file from S3 and returns a writer containing the file contents and metadata
// The path should be in the format <env>/<group>/<team>/<fileName>
// Metadata will include X-File-Content-Type and X-File-Checksum headers if available
func (s *S3FileDownloader) DownloadFile(ctx context.Context, path string) (*io.Writer, map[string]string, error) {
	// Get logger from context first, falling back to the configured logger if not available
	log := logr.FromContextOrDiscard(ctx)

	// Validate client initialization
	if err := s.wrapper.ValidateClient(ctx); err != nil {
		return nil, nil, err
	}

	// Update credentials using token from context if available
	s.wrapper.UpdateCredentialsFromContext(ctx)

	log.V(1).Info("Downloading file", "path", path, "bucket", s.config.BucketName)

	// Get object info for metadata
	objInfo, err := s.wrapper.GetObjectInfo(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	// Download the object data
	buf, err := s.downloadObject(ctx, path)
	if err != nil {
		return nil, nil, err
	}

	// Extract metadata from object info
	metadata := s.wrapper.ExtractMetadata(ctx, objInfo)

	log.V(1).Info("File downloaded successfully", "path", path)

	// Create a writer from the buffer
	var writer io.Writer = buf
	return &writer, metadata, nil
}
