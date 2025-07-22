// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
	"io"
)

type UploadController interface {
	UploadFile(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error)
}

type uploadController struct {
	FileUploader backend.FileUploader
}

func NewUploadController(fu backend.FileUploader) UploadController {
	return &uploadController{FileUploader: fu}
}

func (u uploadController) UploadFile(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error) {
	// Validate fileId format first
	if err := identifier.ValidateFileID(fileId); err != nil {
		return "", errors.Wrap(err, "invalid fileId")
	}

	// Use the fileUploader to upload the file with metadata
	return u.FileUploader.UploadFile(ctx, fileId, file, metadata)
}
