// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package obfuscator

import (
	"testing"
)

func TestObfuscation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ISO 8601 timestamp",
			input:    "Build Time: 2025-10-22T12:54:07Z",
			expected: "Build Time: TIMESTAMP",
		},
		{
			name:     "Log timestamp",
			input:    "2025-10-26T12:37:19.581+0100 INFO message",
			expected: "LOG_TIMESTAMP INFO message",
		},
		{
			name:     "Long git commit hash",
			input:    "Git Commit: dc884a13586fe077c380e2240fc4b7870d49bf3b",
			expected: "Git Commit: COMMIT_HASH",
		},
		{
			name:     "Short git commit hash",
			input:    "Commit: gdc884a1-dirty",
			expected: "Commit: SHORT_HASH",
		},
		{
			name:     "Version string",
			input:    "Version: v0.14.0-13-gdc884a1-dirty",
			expected: "Version: VERSION",
		},
		{
			name:     "Duration",
			input:    "duration: 9.4372ms",
			expected: "duration: DURATION",
		},
		{
			name:     "Token generation",
			input:    "Token: 1761134582 (generated 3 day(s) ago)",
			expected: "Token: 1761134582 (generated X day(s) ago)",
		},
		{
			name:     "Multiple patterns",
			input:    "2025-10-26T12:37:19.581+0100 Version: v0.14.0-13-gdc884a1-dirty (build-time: 2025-10-22T12:54:07Z, git-commit: dc884a13586fe077c380e2240fc4b7870d49bf3b)",
			expected: "LOG_TIMESTAMP Version: VERSION (build-time: TIMESTAMP, git-commit: COMMIT_HASH)",
		},
		{
			name:     "Version without git hash",
			input:    "Version: v1.2.3-5",
			expected: "Version: VERSION",
		},
		{
			name:     "Version without dirty flag",
			input:    "Version: v1.2.3-5-gabcdef1",
			expected: "Version: VERSION",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ObfuscateSnapshot([]byte(tc.input))
			if err != nil {
				t.Fatalf("ObfuscateSnapshot() error = %v", err)
			}
			if string(result) != tc.expected {
				t.Errorf("ObfuscateSnapshot() = %q, want %q", string(result), tc.expected)
			}
		})
	}
}
