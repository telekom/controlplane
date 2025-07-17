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

type DownloadResponse struct {
	Contents string `json:"contents"`
}

type DownloadController interface {
	DownloadFile(ctx context.Context, fileId string) (*io.Writer, error)
}

type downloadController struct {
	FileDownloader backend.FileDownloader
}

func NewDownloadController(fd backend.FileDownloader) DownloadController {
	return &downloadController{FileDownloader: fd}
}

func (d downloadController) DownloadFile(ctx context.Context, fileId string) (*io.Writer, error) {
	// Validate fileId format first
	if err := ValidateFileID(fileId); err != nil {
		return nil, errors.Wrap(err, "invalid fileId")
	}

	// Use the fileDownloader to download the file
	return d.FileDownloader.DownloadFile(ctx, fileId)
}
