// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

type UploadController interface {
	UploadFile(ctx context.Context, fileId string, file *io.Reader) (string, error)
}

type uploadController struct {
	FileUploader backend.FileUploader
}

func NewUploadController(fu backend.FileUploader) UploadController {
	return &uploadController{FileUploader: fu}
}

func (u uploadController) UploadFile(ctx context.Context, fileId string, file *io.Reader) (string, error) {
	// Validate fileId format first
	if err := ValidateFileID(fileId); err != nil {
		return "", errors.Wrap(err, "invalid fileId")
	}

	// Use the fileUploader to upload the file
	return u.FileUploader.UploadFile(ctx, fileId, file)
}
