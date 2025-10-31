// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package obfuscator

import (
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
)

// Common obfuscation patterns for e2e tests
var (
	// DefaultPatterns contains the default obfuscation patterns for e2e test snapshots
	DefaultPatterns = []obfuscator.ObfuscationTarget{
		// ISO 8601 timestamps (e.g., 2025-10-22T12:54:07Z)
		{
			Pattern: `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`,
			Replace: "TIMESTAMP",
		},
		// Log timestamps (e.g., 2025-10-26T12:37:19.581+0100)
		{
			Pattern: `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+[+-]\d{4}`,
			Replace: "LOG_TIMESTAMP",
		},
		// Git commit hashes (long form: 40 characters)
		{
			Pattern: `([a-f0-9]{40})`,
			Replace: "COMMIT_HASH",
		},
		// Specific version patterns with commit hashes (MUST be before SHORT_HASH pattern)
		{
			Pattern: `v\d+\.\d+\.\d+(-\d+)?(-g[a-f0-9]{7,12})?(-dirty)?`,
			Replace: "VERSION",
		},
		// Git commit hashes (short form: 7-12 characters in typical git output contexts)
		{
			Pattern: `(g[a-f0-9]{7,12})(-dirty)?`,
			Replace: "SHORT_HASH",
		},
		// Duration measurements
		{
			Pattern: `(\d+\.\d+)(ms|s|m)`,
			Replace: "DURATION",
		},
		// Token generation times
		{
			Pattern: `generated (.+ ago|yesterday)`, // matches the info-output like "generated 5m ago" or "generated yesterday"
			Replace: "generated X ago",
		},
		// Any ClientSecret
		{
			Pattern: `irisClientSecret: .*`,
			Replace: "irisClientSecret: OBFUSCATED",
		},
	}
)

// ObfuscateSnapshot applies obfuscation to the given byte array
func ObfuscateSnapshot(data []byte) ([]byte, error) {
	return obfuscator.ObfuscateBytes(data, DefaultPatterns...)
}
