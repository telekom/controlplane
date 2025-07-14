package controller

import (
	"context"
	"github.com/telekom/controlplane/file-manager/pkg/backend"
	"io"
)

type DownloadResponse struct {
	Contents string `json:"contents"`
}

type DownloadController interface {
	DownloadFile(ctx context.Context, env string, group string, team string, fileId string) (*io.Writer, error)
}

type downloadController struct {
	FileDownloader backend.FileDownloader
}

func NewDownloadController(fd backend.FileDownloader) DownloadController {
	return &downloadController{FileDownloader: fd}
}

func (d downloadController) DownloadFile(ctx context.Context, env string, group string, team string, fileId string) (*io.Writer, error) {
	//TODO implement me
	panic("implement me")
}
