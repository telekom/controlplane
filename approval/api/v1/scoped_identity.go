// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ScopedIntentHash deterministically encodes a JSON-compatible value into a
// hex SHA-256 digest. The encoding normalises object-key ordering by
// round-tripping through json.Decoder (UseNumber) and re-marshalling, so map
// insertion order does not affect the result. Array order and type distinctions
// (string "1" vs number 1) are preserved.
//
// Returns an error for values that cannot be marshalled to JSON.
func ScopedIntentHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("scoped intent hash: marshal: %w", err)
	}

	// Round-trip to normalise key ordering.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var normalised any
	if err := dec.Decode(&normalised); err != nil {
		return "", fmt.Errorf("scoped intent hash: decode: %w", err)
	}

	canonical, err := json.Marshal(normalised)
	if err != nil {
		return "", fmt.Errorf("scoped intent hash: re-marshal: %w", err)
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ScopedIdentityMatch returns true when two (target, key) pairs refer to the
// same scoped identity. It compares API group (not full APIVersion), kind,
// namespace, name, UID and key. A served-version difference within the same
// group/kind is treated as the same identity.
func ScopedIdentityMatch(a, b types.TypedObjectRef, keyA, keyB string) bool {
	if keyA != keyB {
		return false
	}
	if a.UID != b.UID {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Namespace != b.Namespace {
		return false
	}
	if a.Name != b.Name {
		return false
	}

	groupA := apiGroup(a.APIVersion)
	groupB := apiGroup(b.APIVersion)
	return groupA == groupB
}

// apiGroup extracts the group component from an APIVersion string.
// Returns empty string for core types (e.g. "v1").
func apiGroup(apiVersion string) string {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return apiVersion
	}
	return gv.Group
}
