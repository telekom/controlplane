package s3

import (
	"context"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

var _ backend.FileUploader = &S3FileUploader{}

type S3FileUploader struct {
	// todo something here
}

func (s S3FileUploader) UploadFile(ctx context.Context, env string, group string, team string, file *io.Reader) (string, error) {
	//TODO implement me
	panic("implement me")
}

func NewS3FileUploader() *S3FileUploader {
	//TODO implement me
	return &S3FileUploader{}
}
