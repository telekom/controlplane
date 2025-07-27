// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package constants

type HeaderName string

var (
	HeaderNameChecksum            HeaderName = "X-File-Checksum"
	HeaderNameOriginalContentType HeaderName = "X-File-Content-Type"
)

func (h HeaderName) String() string {
	return string(h)
}
