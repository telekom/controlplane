// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared

import "strings"

// TeamNameFromNamespace extracts the composite team name from a namespace
// following the convention "<env>--<group>--<team>".
//
// The Team CR metadata.name is "<group>--<team>" (e.g. "eni--narvi-regr"),
// so we drop the first segment (environment) and return everything after
// the first "--" separator. If no "--" is found the full namespace is
// returned as-is.
func TeamNameFromNamespace(namespace string) string {
	if idx := strings.Index(namespace, "--"); idx >= 0 {
		return namespace[idx+2:]
	}
	return namespace
}
