// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"context"
	"strings"
	"testing"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
)

func TestExecutor_Execute(t *testing.T) {
	// Create a test configuration
	roverCtlConfig := config.RoverCtlConfig{
		Binary: "echo", // Use echo as a simple command to test
	}
	environment := config.Environments{
		Name:  "test-env",
		Token: "test-token",
	}

	// Create the executor
	executor := NewExecutor(roverCtlConfig, environment)

	// Test cases
	tests := []struct {
		name           string
		command        string
		expectError    bool
		expectedStdout string
	}{
		{
			name:           "Simple echo command",
			command:        "Hello, world!",
			expectError:    false,
			expectedStdout: "Hello, world!",
		},
		{
			name:           "Multiple arguments",
			command:        "arg1 arg2 arg3",
			expectError:    false,
			expectedStdout: "arg1 arg2 arg3",
		},
	}

	// Run test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := executor.Execute(ctx, tc.command, nil)

			// Check error
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Check stdout
			if !strings.Contains(result.Stdout, tc.expectedStdout) {
				t.Errorf("Expected stdout to contain '%s', but got '%s'", tc.expectedStdout, result.Stdout)
			}

			// Check exit code
			if result.ExitCode != 0 {
				t.Errorf("Expected exit code 0, but got %d", result.ExitCode)
			}
		})
	}
}

func TestExecutor_CreateSnapshot(t *testing.T) {
	// Create a test configuration
	roverCtlConfig := config.RoverCtlConfig{
		Binary: "echo",
	}
	environment := config.Environments{
		Name:  "test-env",
		Token: "test-token",
	}

	// Create the executor
	executor := NewExecutor(roverCtlConfig, environment)

	// Create a test result
	execResult := &ExecuteResult{
		ExitCode: 0,
		Stdout:   "Test stdout",
		Stderr:   "Test stderr",
		Duration: 0,
	}

	// Create a snapshot
	cmdStr := "test command"
	suiteName := "test-suite"
	caseIndex := "0"
	caseName := "test-case"
	snapshot := executor.CreateSnapshot(cmdStr, execResult, "", suiteName, caseIndex, caseName)

	// Verify snapshot
	if snapshot.Id == "" {
		t.Errorf("Expected snapshot ID to be non-empty")
	}
	if snapshot.Output.Command != cmdStr {
		t.Errorf("Expected command to be '%s', but got '%s'", cmdStr, snapshot.Output.Command)
	}
	if snapshot.Output.ExitCode != execResult.ExitCode {
		t.Errorf("Expected exit code to be %d, but got %d", execResult.ExitCode, snapshot.Output.ExitCode)
	}
	if snapshot.Output.Stdout != execResult.Stdout {
		t.Errorf("Expected stdout to be '%s', but got '%s'", execResult.Stdout, snapshot.Output.Stdout)
	}
	if snapshot.Output.Stderr != execResult.Stderr {
		t.Errorf("Expected stderr to be '%s', but got '%s'", execResult.Stderr, snapshot.Output.Stderr)
	}
	if snapshot.Output.Environment != environment.Name {
		t.Errorf("Expected environment to be '%s', but got '%s'", environment.Name, snapshot.Output.Environment)
	}
}
