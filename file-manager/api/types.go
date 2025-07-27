// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

type FileUploadResponse struct {
	MD5Hash     string
	FileId      string
	ContentType string
}

type FileDownloadResponse struct {
	MD5Hash     string
	ContentType string
	Content     []byte
}
