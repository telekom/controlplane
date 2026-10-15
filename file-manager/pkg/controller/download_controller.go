// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"io"

	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"github.com/telekom/controlplane/file-manager/pkg/backend/identifier"
)

type DownloadResponse struct {
	Contents string `json:"contents"`
}

type DownloadController interface {
	DownloadFile(ctx context.Context, fileId string) (io.Reader, map[string]string, error)
}

type downloadController struct {
	FileDownloader backend.FileDownloader
}

func NewDownloadController(fd backend.FileDownloader) DownloadController {
	return &downloadController{FileDownloader: fd}
}

func (d downloadController) DownloadFile(ctx context.Context, fileId string) (io.Reader, map[string]string, error) {
	// Validate fileId format first
	// TODO: backward-compatibility: uncomment it after all files were migrated to UUID
	// if err := identifier.ValidateFileID(fileId); err != nil {
	// 	return nil, nil, backend.ErrInvalidFileId(fileId)
	// }

	return d.FileDownloader.DownloadFile(ctx, identifier.ConvertFileIdToPath(fileId))
}
