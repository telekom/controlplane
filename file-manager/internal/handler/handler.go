// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/internal/api"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"io"
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
	fileId := request.FileId
	fileData := request.Body

	if fileData == nil {
		return nil, errors.New("no file data provided")
	}

	// Use the controller to upload the file
	id, err := h.ctrl.UploadFile(ctx, fileId, &fileData)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to upload file with ID %s", fileId))
	}

	// Return the successful response
	return api.UploadFile200JSONResponse{
		FileUploadResponseJSONResponse: api.FileUploadResponseJSONResponse{
			Id: id,
		},
	}, nil
}

func (h *Handler) DownloadFile(ctx context.Context, request api.DownloadFileRequestObject) (res api.DownloadFileResponseObject, err error) {
	fileId := request.FileId

	// Use the controller to download the file
	fileData, err := h.ctrl.DownloadFile(ctx, fileId)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to download file with ID %s", fileId))
	}

	if fileData == nil {
		return nil, errors.New("file not found or empty")
	}

	// Convert the writer to a reader for the response
	writer := *fileData
	var reader io.Reader

	// If it's a type that can be converted to a reader
	if readWriter, ok := writer.(io.ReadWriter); ok {
		reader = readWriter
	} else {
		return nil, errors.New("could not convert file data to readable format")
	}

	// Return the successful response
	return api.DownloadFile200ApplicationoctetStreamResponse{
		FileDownloadResponseApplicationoctetStreamResponse: api.FileDownloadResponseApplicationoctetStreamResponse{
			Body: reader,
		},
	}, nil
}
