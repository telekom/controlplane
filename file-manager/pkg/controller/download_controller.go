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

type DownloadResponse struct {
	Contents string `json:"contents"`
}

type DownloadController interface {
	DownloadFile(ctx context.Context, fileId string) (*io.Writer, map[string]string, error)
}

type downloadController struct {
	FileDownloader backend.FileDownloader
}

func NewDownloadController(fd backend.FileDownloader) DownloadController {
	return &downloadController{FileDownloader: fd}
}

func (d downloadController) DownloadFile(ctx context.Context, fileId string) (*io.Writer, map[string]string, error) {
	// Validate fileId format first
	if err := identifier.ValidateFileID(fileId); err != nil {
		return nil, nil, errors.Wrap(err, "invalid fileId")
	}

	// Convert fileId to S3 path format
	s3Path, err := identifier.ConvertFileIdToPath(fileId)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to convert fileId to path")
	}

	// Use the fileDownloader to download the file using the converted path
	return d.FileDownloader.DownloadFile(ctx, s3Path)
}
