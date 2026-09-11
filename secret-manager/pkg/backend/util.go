// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
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
	return NoSubPath
}

func GetPath(secretPath string) string {
	parts := strings.SplitN(secretPath, "/", 2)
	if len(parts) > 1 {
		return parts[0]
	}
	return secretPath
}

// ShallowMergeJSON attempts to shallow-merge two JSON object strings.
// If both current and incoming are valid JSON objects, it merges incoming keys
// into current (incoming keys overwrite, existing keys not in incoming are preserved).
// Returns the merged JSON string and true if merging was performed,
// or the incoming value and false if either value is not a JSON object.
func ShallowMergeJSON(current, incoming string) (string, bool) {
	var currentMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(current), &currentMap); err != nil {
		return incoming, false
	}
	var incomingMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(incoming), &incomingMap); err != nil {
		return incoming, false
	}

	// Shallow merge: incoming keys overwrite, existing keys are preserved
	maps.Copy(currentMap, incomingMap)

	merged, err := json.Marshal(currentMap)
	if err != nil {
		return incoming, false
	}
	return string(merged), true
}
