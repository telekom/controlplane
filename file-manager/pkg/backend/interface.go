package backend

import (
	"context"
	"io"
)

type FileUploader interface {
	UploadFile(ctx context.Context, env string, group string, team string, file *io.Reader) (string, error)
}

type FileDownloader interface {
	DownloadFile(ctx context.Context, env string, group string, team string, fileId string) (*io.Writer, error)
}
