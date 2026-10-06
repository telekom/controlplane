// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import (
	"strings"
)

// ConvertFileIdToPath maps legacy IDs to their object key and leaves UUIDs unchanged.
func ConvertFileIdToPath(fileId string) string {
	parts := strings.SplitN(fileId, "--", 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return fileId
	}
	return strings.Join(parts, "/")
}
