// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
)

func MakeName(ownerName, basePath, organization string) string {
	name := ownerName + "--" + strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
	if organization != "" {
		name = organization + "--" + name
	}

	return name
}
