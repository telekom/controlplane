// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package subscriber

import (
	"crypto/sha1" // #nosec G505 -- SHA-1 used for deterministic ID generation, not security
	"encoding/hex"
	"strings"
)

// GenerateSubscriptionID generates a deterministic subscription ID by computing
// the SHA-1 hash of "environment--eventType--subscriberId".
func GenerateSubscriptionID(environment, eventType, subscriberID string) string {
	data := strings.Join([]string{environment, eventType, subscriberID}, "--")
	hash := sha1.Sum([]byte(data)) // #nosec G401 -- SHA-1 used for deterministic ID generation, not security
	return hex.EncodeToString(hash[:])
}
