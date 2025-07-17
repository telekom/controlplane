package backend

import (
	"context"
	"io"
)

type FileUploader interface {
	UploadFile(ctx context.Context, fileId string, file *io.Reader) (string, error)
}

type FileDownloader interface {
	DownloadFile(ctx context.Context, fileId string) (*io.Writer, error)
}
