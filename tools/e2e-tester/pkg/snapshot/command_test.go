// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"strings"
	"testing"
)

func TestCommandSnapshot_String_MultilineFormat(t *testing.T) {
	// Test case with single-line output
	singleLineSnap := &CommandSnapshot{
		Id:      "test-single-line",
		Version: 1,
		Output: CommandOutput{
			Command:     "echo test",
			ExitCode:    0,
			Stdout:      "simple output",
			Stderr:      "simple error",
			Environment: "test-env",
			Duration:    "1ms",
		},
	}

	// Test case with multi-line output
	multiLineSnap := &CommandSnapshot{
		Id:      "test-multi-line",
		Version: 1,
		Output: CommandOutput{
			Command:     "echo test",
			ExitCode:    0,
			Stdout:      "line 1\nline 2\nline 3",
			Stderr:      "error line 1\nerror line 2",
			Environment: "test-env",
			Duration:    "1ms",
		},
	}

	// Get string representation of single-line snapshot
	singleLineStr := singleLineSnap.String()

	// Single line output should not have indented format
	if strings.Contains(singleLineStr, "\n  line") {
		t.Errorf("Single line stdout should not be formatted as multiline: %s", singleLineStr)
	}

	// Get string representation of multi-line snapshot
	multiLineStr := multiLineSnap.String()

	// Multi-line output should use indented format with the YAML literal style
	if !strings.Contains(multiLineStr, "|-") && !strings.Contains(multiLineStr, "\n  line") {
		t.Errorf("Multi-line stdout should be formatted with proper YAML multiline style: %s", multiLineStr)
	}

	// Verify stderr is also properly formatted
	if !strings.Contains(multiLineStr, "|-") && !strings.Contains(multiLineStr, "\n  error") {
		t.Errorf("Multi-line stderr should be formatted with proper YAML multiline style: %s", multiLineStr)
	}
}
