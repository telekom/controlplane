// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"fmt"
	"strings"
)

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
