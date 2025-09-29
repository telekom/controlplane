// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/file-manager/pkg/backend"
)

type Controller interface {
	UploadController
	DownloadController
	DeleteController
}

type controller struct {
	UploadController
	DownloadController
	DeleteController
}

func NewController(fd backend.FileDownloader, fu backend.FileUploader, del backend.FileDeleter) Controller {
	if del == nil {
		del = nil // use default deleter
	}
	return &controller{
		UploadController:   NewUploadController(fu),
		DownloadController: NewDownloadController(fd),
		DeleteController:   NewDeleteController(del),
	}
}
