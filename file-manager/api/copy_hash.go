// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
)

// copyAndHash copies data from src to dst while also computing a hash of the data.
// It returns the computed hash as a base64 encoded string,
// and any error encountered during the copy operation.
func copyAndHash(dst io.Writer, src io.Reader) (size int64, hash string, err error) {
	hasher := sha256.New()

	multiWriter := io.MultiWriter(dst, hasher)

	size, err = io.Copy(multiWriter, src)
	if err != nil {
		return size, "", err
	}

	hash = base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	return size, hash, nil
}
