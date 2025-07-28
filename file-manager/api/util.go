// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/md5"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/telekom/controlplane/file-manager/pkg/constants"
)

func Md5Base64(reader io.Reader) (string, error) {
	hasher := md5.New()
	// Copy the entire reader into the hasher
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}

	// Compute MD5 sum
	sum := hasher.Sum(nil)

	// Return base64-encoded string of the MD5 hash (S3 wants this format)
	return base64.StdEncoding.EncodeToString(sum), nil
}

func stringPtr(s string) *string {
	return &s
}

func extractHeader(httpResponse *http.Response, checksum constants.HeaderName) string {
	value := httpResponse.Header.Get(checksum.String())
	if value == "" {
		return "undefined"
	}
	return value
}
