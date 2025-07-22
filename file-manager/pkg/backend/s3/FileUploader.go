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
func (s *S3FileUploader) UploadFile(ctx context.Context, fileId string, reader *io.Reader) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	if s.config == nil || s.config.Client == nil {
		log.Error(nil, "S3 client not initialized")
		return "", errors.New("S3 client not initialized")
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

	// Get content type - we'll use application/octet-stream as default
	contentType := "application/octet-stream"

	// Upload file using the S3 path instead of fileId directly
	log.V(1).Info("Starting S3 PutObject operation")
	_, err = s.config.Client.PutObject(ctx, s.config.BucketName, s3Path, *reader, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})

	if err != nil {
		log.Error(err, "Failed to upload file to S3")
		return "", errors.Wrap(err, "failed to upload file")
	}
	log.V(1).Info("File uploaded successfully", "fileId", fileId, "s3Path", s3Path)

	// Return the original fileId as that's the expected return value by the interface
	return fileId, nil
}
