// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package s3

import (
	"bytes"
	"context"
	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

var _ backend.FileDownloader = &S3FileDownloader{}

type S3FileDownloader struct {
	config *S3Config
}

// NewS3FileDownloader creates a new S3FileDownloader with the given configuration
func NewS3FileDownloader(config *S3Config) *S3FileDownloader {
	return &S3FileDownloader{
		config: config,
	}
}

// DownloadFile downloads a file from S3 and returns a writer containing the file contents
// The fileId should follow the convention <env>--<group>--<team>--<fileName>
// The file will be retrieved from S3 using a path format: <env>/<group>/<team>/<fileName>
func (s *S3FileDownloader) DownloadFile(ctx context.Context, fileId string) (*io.Writer, error) {
	// Get logger from context first, falling back to the configured logger if not available
	log := logr.FromContextOrDiscard(ctx)

	if s.config == nil || s.config.Client == nil {
		log.Error(nil, "S3 client not initialized")
		return nil, errors.New("S3 client not initialized")
	}

	// Convert fileId to S3 path format
	s3Path, err := ConvertFileIdToS3Path(fileId)
	if err != nil {
		log.Error(err, "Failed to convert fileId to S3 path")
		return nil, errors.Wrap(err, "failed to convert fileId to S3 path")
	}

	log.V(1).Info("Downloading file", "fileId", fileId, "s3Path", s3Path, "bucket", s.config.BucketName)

	// Get object from S3 using the S3 path instead of fileId directly
	log.V(1).Info("Starting S3 GetObject operation")
	object, err := s.config.Client.GetObject(ctx, s.config.BucketName, s3Path, minio.GetObjectOptions{})
	if err != nil {
		log.Error(err, "Failed to get file from S3")
		return nil, errors.Wrap(err, "failed to get file from S3")
	}
	defer object.Close()

	// Create a buffer to store the downloaded content
	buf := new(bytes.Buffer)

	// Copy object data to buffer
	_, err = io.Copy(buf, object)
	if err != nil {
		log.Error(err, "Failed to read file data")
		return nil, errors.Wrap(err, "failed to read file data")
	}

	log.V(1).Info("File downloaded successfully", "fileId", fileId, "s3Path", s3Path)

	// Create a writer from the buffer
	var writer io.Writer = buf
	return &writer, nil
}
