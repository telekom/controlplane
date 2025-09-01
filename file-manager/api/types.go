// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

type FileUploadResponse struct {
	FileHash    string
	FileId      string
	ContentType string
}

type FileDownloadResponse struct {
	FileHash    string
	ContentType string
}
