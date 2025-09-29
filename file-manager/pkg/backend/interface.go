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
	// The fileId should follow the convention <env>--<group>--<team>--<fileName>
	UploadFile(ctx context.Context, fileId string, file io.Reader, metadata map[string]string) (string, error)
}

type FileDownloader interface {
	// DownloadFile downloads a file with the given fileId and returns the file content along with metadata
	// The metadata map can contain content type (X-File-Content-Type) and checksum (X-File-Checksum) values
	DownloadFile(ctx context.Context, fileId string) (io.Reader, map[string]string, error)
}

type FileDeleter interface {
	// DeleteFile deletes a file with the given fileId
	// The fileId should follow the convention <env>--<group>--<team>--<fileName>
	DeleteFile(ctx context.Context, fileId string) error
}
