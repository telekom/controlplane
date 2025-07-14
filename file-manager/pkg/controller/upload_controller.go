package controller

import (
	"context"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

type UploadController interface {
	UploadFile(ctx context.Context, env string, group string, team string, file *io.Reader) (string, error)
}

type uploadController struct {
	FileUploader backend.FileUploader
}

func NewUploadController(fu backend.FileUploader) UploadController {
	return &uploadController{FileUploader: fu}
}

func (u uploadController) UploadFile(ctx context.Context, env string, group string, team string, file *io.Reader) (string, error) {
	//TODO implement me
	panic("implement me")
}
