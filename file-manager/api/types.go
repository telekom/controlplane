// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

type FileUploadResponse struct {
	CRC64NVMEHash string
	FileId        string
	ContentType   string
}

type FileDownloadResponse struct {
	CRC64NVMEHash string
	ContentType   string
}
