package handler

import (
	"context"
	"github.com/telekom/controlplane/file-manager/internal/api"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
)

var _ api.StrictServerInterface = &Handler{}

type Handler struct {
	ctrl controller.Controller
}

func (h Handler) UploadFile(ctx context.Context, request api.UploadFileRequestObject) (api.UploadFileResponseObject, error) {
	//TODO implement me
	panic("implement me")
}

func (h Handler) DownloadFile(ctx context.Context, request api.DownloadFileRequestObject) (api.DownloadFileResponseObject, error) {
	//TODO implement me
	panic("implement me")
}
