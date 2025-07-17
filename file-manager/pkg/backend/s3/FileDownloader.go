package s3

import (
	"context"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

var _ backend.FileDownloader = &S3FileDownloader{}

type S3FileDownloader struct {
	// todo something here
}

func NewS3FileDownloader() *S3FileDownloader {
	//TODO implement me
	return &S3FileDownloader{}
}

func (s S3FileDownloader) DownloadFile(ctx context.Context, fileId string) (*io.Writer, error) {
	//TODO implement me
	panic("implement me")
}
