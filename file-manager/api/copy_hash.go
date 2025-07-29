// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/base64"
	"hash"
	"io"
)

// copyAndHash copies data from src to dst while also computing a hash of the data.
// It returns the number of bytes written, the computed hash as a base64 encoded string,
// and any error encountered during the copy operation.
func copyAndHash(dst io.Writer, src io.Reader, hasher hash.Hash) (written int64, hash string, err error) {
	multiWriter := io.MultiWriter(dst, hasher)

	written, err = io.Copy(multiWriter, src)
	if err != nil {
		return 0, "", err
	}

	sum := hasher.Sum(nil)
	hash = base64.StdEncoding.EncodeToString(sum)

	return written, hash, nil
}
