// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/file-manager/api/constants"
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
	log := logr.FromContextOrDiscard(ctx)

	fileId := request.FileId
	fileData := request.Body

	if fileData == nil {
		return nil, errors.New("no file data provided")
	}

	// debug the file contents

	var buf bytes.Buffer // buffer to store a copy
	tee := io.TeeReader(fileData, &buf)

	// Now read from tee instead of originalReader
	data, err := io.ReadAll(tee)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("cannot copy teeReader"))
	}

	// Now `data` has the contents read (same as originalReader)
	// `buf.Bytes()` contains an exact copy of what was read

	log.V(1).Info("Debug file contents", "contents", string(buf.String()))
	fileData = bytes.NewReader(data)
	// end debug

	// Extract metadata headers from request
	metadata := make(map[string]string)
	if request.Params.XFileContentType != nil {
		metadata[constants.XFileContentType] = *request.Params.XFileContentType
	}
	if request.Params.XFileChecksum != nil {
		metadata[constants.XFileChecksum] = *request.Params.XFileChecksum
	}

	// Use the controller to upload the file with metadata
	id, err := h.ctrl.UploadFile(ctx, fileId, &fileData, metadata)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to upload file with ID %s", fileId))
	}

	// Build response with same headers
	response := api.UploadFile200JSONResponse{
		FileUploadResponseJSONResponse: api.FileUploadResponseJSONResponse{
			Body:    api.FileUploadResponse{Id: id},
			Headers: api.FileUploadResponseResponseHeaders{},
		},
	}

	// Add headers to response if they were provided in the request
	if request.Params.XFileContentType != nil {
		response.Headers.XFileContentType = *request.Params.XFileContentType
	}
	if request.Params.XFileChecksum != nil {
		response.Headers.XFileChecksum = *request.Params.XFileChecksum
	}

	return response, nil
}

func (h *Handler) DownloadFile(ctx context.Context, request api.DownloadFileRequestObject) (res api.DownloadFileResponseObject, err error) {
	fileId := request.FileId

	// Use the controller to download the file
	fileData, metadata, err := h.ctrl.DownloadFile(ctx, fileId)
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

	// Create response with file data
	response := api.DownloadFile200ApplicationoctetStreamResponse{
		FileDownloadResponseApplicationoctetStreamResponse: api.FileDownloadResponseApplicationoctetStreamResponse{
			Body:    reader,
			Headers: api.FileDownloadResponseResponseHeaders{},
		},
	}

	// Add headers to response from metadata
	if contentType, ok := metadata[constants.XFileContentType]; ok && contentType != "" {
		response.Headers.XFileContentType = contentType
	}
	if checksum, ok := metadata[constants.XFileChecksum]; ok && checksum != "" {
		response.Headers.XFileChecksum = checksum
	}

	return response, nil
}
