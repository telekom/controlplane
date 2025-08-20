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
}

type controller struct {
	UploadController
	DownloadController
}

func NewController(fd backend.FileDownloader, fu backend.FileUploader) Controller {
	return &controller{
		UploadController:   NewUploadController(fu),
		DownloadController: NewDownloadController(fd),
	}
}
