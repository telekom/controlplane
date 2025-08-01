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
// It returns the number of bytes written, the computed hash as a base64 encoded string,
// and any error encountered during the copy operation.
func copyAndHash(dst io.Writer, src io.Reader) (written int, hash string, err error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return 0, "", err
	}

	written, err = dst.Write(data)
	if err != nil {
		return 0, "", err
	}

	sum := crc64nvme.Checksum(data)
	sumBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sumBytes, sum)
	hash = base64.StdEncoding.EncodeToString(sumBytes)

	return written, hash, nil
}
