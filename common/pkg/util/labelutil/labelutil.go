// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package labelutil

import "strings"

var (
	unwantedChars = []string{" ", "", "_", "-", "/", "-", "\\", "-"}
	replacer      = strings.NewReplacer(unwantedChars...)
)

func NormalizeValue(value string) string {
	value = strings.ToLower(replacer.Replace(value))
	return strings.Trim(value, "-")
}
