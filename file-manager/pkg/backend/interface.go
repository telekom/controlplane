// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"io"
)

type FileUploader interface {
	// UploadFile uploads a file with the given fileId, content, and optional metadata
	// The metadata map can contain content type (X-File-Content-Type) and checksum (X-File-Checksum) values
	UploadFile(ctx context.Context, fileId string, file *io.Reader, metadata map[string]string) (string, error)
}

type FileDownloader interface {
	DownloadFile(ctx context.Context, fileId string) (*io.Writer, error)
}
