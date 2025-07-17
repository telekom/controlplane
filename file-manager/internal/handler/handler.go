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

func NewHandler(ctrl controller.Controller) *Handler {
	return &Handler{
		ctrl: ctrl,
	}
}

func (h *Handler) UploadFile(ctx context.Context, request api.UploadFileRequestObject) (res api.UploadFileResponseObject, err error) {

	//TODO implement me
	panic("implement me")
}

func (h *Handler) DownloadFile(ctx context.Context, request api.DownloadFileRequestObject) (res api.DownloadFileResponseObject, err error) {
	//TODO implement me
	panic("implement me")
}
