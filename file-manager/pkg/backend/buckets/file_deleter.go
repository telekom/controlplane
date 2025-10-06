// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package buckets

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/minio/minio-go/v7"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

var _ backend.FileDeleter = &BucketFileDeleter{}

type BucketFileDeleter struct {
	config  *BucketConfig
	wrapper *MinioWrapper
}

func NewBucketFileDeleter(config *BucketConfig) *BucketFileDeleter {
	return &BucketFileDeleter{
		config:  config,
		wrapper: NewMinioWrapper(config),
	}
}

// DeleteFile deletes a file from bucket
// The path should be in the format <env>/<group>/<team>/<fileName>
func (d *BucketFileDeleter) DeleteFile(ctx context.Context, path string) error {
	// Get logger from context first, falling back to the configured logger if not available
	log := logr.FromContextOrDiscard(ctx)

	// Validate client initialization
	if err := d.wrapper.ValidateClient(ctx); err != nil {
		return err
	}

	log.V(1).Info("Deleting file", "path", path, "bucket", d.config.BucketName)

	// Check existence to map 404 semantics explicitly
	if _, err := d.wrapper.GetObjectInfo(ctx, path); err != nil {
		return backend.ErrFileNotFound(path)
	}

	// Perform delete
	err := d.config.Client.RemoveObject(ctx, d.config.BucketName, path, minio.RemoveObjectOptions{})
	if err != nil {
		return backend.NewBackendError(path, err, "DeleteFailed")
	}
	log.V(1).Info("File deleted", "path", path)
	return nil
}
