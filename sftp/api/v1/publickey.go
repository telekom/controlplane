// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
)

const sshFingerprintPrefix = "SHA256:"

// FingerprintForKey returns the OpenSSH-style SHA256 fingerprint for an authorized_keys public key.
func FingerprintForKey(key string) (string, error) {
	_, key, err := publicKeyFields(key)
	if err != nil {
		return "", err
	}

	blob, err := decodeAuthorizedKeyBlob(key)
	if err != nil {
		return "", fmt.Errorf("decoding SSH public key: %w", err)
	}

	sum := sha256.Sum256(blob)
	builder := &strings.Builder{}
	builder.Grow(len(sshFingerprintPrefix) + sha256.Size*2)
	builder.WriteString(sshFingerprintPrefix)
	hexWriter := hex.NewEncoder(builder)
	_, err = hexWriter.Write(sum[:])
	if err != nil {
		return "", fmt.Errorf("converting hash to hex failed: %w", err)
	}

	return builder.String(), nil
}

// CanonicalPublicKey returns the authorized_keys public key without any trailing comment.
func CanonicalPublicKey(key string) (string, error) {
	keyType, encoded, err := publicKeyFields(key)
	if err != nil {
		return "", err
	}
	return keyType + " " + encoded, nil
}

func publicKeyFields(key string) (string, string, error) {
	fields := strings.Fields(key)
	if len(fields) < 2 {
		return "", "", fmt.Errorf("invalid SSH public key")
	}
	return fields[0], fields[1], nil
}

func decodeAuthorizedKeyBlob(encoded string) ([]byte, error) {
	if blob, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return blob, nil
	}
	return base64.RawStdEncoding.DecodeString(encoded)
}

// SourceForTypedObjectRef returns a stable claim source string for a typed object reference.
func SourceForTypedObjectRef(ref types.TypedObjectRef) string {
	return strings.ToLower(ref.Kind + "." + ref.APIVersion + "/" + ref.Namespace + "/" + ref.Name)
}
