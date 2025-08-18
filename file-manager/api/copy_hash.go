// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/base64"
	"encoding/binary"
	"io"

	"github.com/minio/crc64nvme"
)

// copyAndHash copies data from src to dst while also computing a hash of the data.
// It returns the computed hash as a base64 encoded string,
// and any error encountered during the copy operation.
func copyAndHash(dst io.Writer, src io.Reader) (size int64, hash string, err error) {
	hasher := crc64nvme.New()

	multiWriter := io.MultiWriter(dst, hasher)

	size, err = io.Copy(multiWriter, src)
	if err != nil {
		return size, "", err
	}

	// Get the hash value and encode it to base64
	hashBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(hashBytes, hasher.Sum64())
	hash = base64.StdEncoding.EncodeToString(hashBytes)

	return size, hash, nil
}
