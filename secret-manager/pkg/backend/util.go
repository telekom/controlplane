// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// MakeChecksum is used to generate a checksum for a given string.
func MakeChecksum(input string) string {
	byteInput := []byte(input)
	hash := sha256.Sum256(byteInput)
	// Use the first 6 bytes of the hash to reduce size
	// Collisions are unlikely
	return hex.EncodeToString(hash[:6])
}

func GetSubPath(secretPath string) string {
	parts := strings.SplitN(secretPath, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func GetPath(secretPath string) string {
	parts := strings.SplitN(secretPath, "/", 2)
	if len(parts) > 1 {
		return parts[0]
	}
	return secretPath
}
