// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package xdsapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"google.golang.org/protobuf/proto"
)

const SchemaVersion = "v1"

// Digest returns the SHA-256 digest of the deterministic protobuf encoding
// with the digest field itself cleared.
func Digest(bundle *Bundle) (string, error) {
	if bundle == nil {
		return "", fmt.Errorf("bundle is required")
	}

	canonical, ok := proto.Clone(bundle).(*Bundle)
	if !ok {
		return "", fmt.Errorf("cloning canonical bundle")
	}
	canonical.Digest = ""
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("marshalling canonical bundle: %w", err)
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// SetDigest calculates and assigns the canonical digest.
func SetDigest(bundle *Bundle) error {
	digest, err := Digest(bundle)
	if err != nil {
		return err
	}
	bundle.Digest = digest
	return nil
}

// MarshalDeterministic serializes a complete envelope for durable storage.
func MarshalDeterministic(bundle *Bundle) ([]byte, error) {
	payload, err := (proto.MarshalOptions{Deterministic: true}).Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("marshalling bundle: %w", err)
	}
	return payload, nil
}
