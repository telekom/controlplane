// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/snapshot"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/util"
	"go.uber.org/zap"
)

// SnapshotExecutor handles executing snapshotter commands and capturing their outputs
type SnapshotExecutor struct {
	snapshotterConfig config.SnapshotterConfig
	environment       config.Environments
}

// NewSnapshotExecutor creates a new snapshotter command executor
func NewSnapshotExecutor(snapshotterConfig config.SnapshotterConfig, environment config.Environments) *SnapshotExecutor {
	return &SnapshotExecutor{
		snapshotterConfig: snapshotterConfig,
		environment:       environment,
	}
}

// Execute runs a snapshotter command and returns its output
func (e *SnapshotExecutor) Execute(ctx context.Context, cmdStr string, params map[string]interface{}) (*ExecuteResult, error) {
	startTime := time.Now()

	var cmd *exec.Cmd
	var fullCmd string

	// Check if we're using a URL or a binary
	if e.snapshotterConfig.URL != "" {
		// HTTP mode - use curl or a similar tool to interact with the snapshotter service
		// For example: curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/snap?route=/api/v1
		apiPath := "snap"
		fullCmd = fmt.Sprintf("curl -s -H \"Authorization: Bearer %s\" \"%s/%s?%s\"",
			e.environment.Token, e.snapshotterConfig.URL, apiPath, cmdStr)
		cmd = exec.CommandContext(ctx, "sh", "-c", fullCmd)
	} else {
		// Binary mode - execute the snapshotter binary directly
		// Get the binary name (use default "snapshotter" if not specified)
		binaryName := "snapshotter"
		if e.snapshotterConfig.Binary != "" {
			binaryName = e.snapshotterConfig.Binary
		}

		// Construct the full command with binary name and command string
		fullCmd = fmt.Sprintf("%s %s", binaryName, cmdStr)
		cmd = exec.CommandContext(ctx, "sh", "-c", fullCmd)

		// Set up environment variables
		if e.environment.Token != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("SNAPSHOTTER_TOKEN=%s", e.environment.Token))
		}
	}

	// Log the full command details for debugging
	if e.snapshotterConfig.URL != "" {
		zap.L().Debug("executing snapshotter command via HTTP",
			zap.String("full_command", fullCmd),
			zap.String("url", e.snapshotterConfig.URL),
			zap.String("api_path", "snap"),
			zap.String("command_args", cmdStr),
			zap.String("environment", e.environment.Name))
	} else {
		// When using binary mode, use the same binary name we used for execution
		binaryName := "snapshotter" // Default if not specified
		if e.snapshotterConfig.Binary != "" {
			binaryName = e.snapshotterConfig.Binary
		}
		zap.L().Debug("executing snapshotter command via binary",
			zap.String("full_command", fullCmd),
			zap.String("binary", binaryName),
			zap.String("command_args", cmdStr),
			zap.String("environment", e.environment.Name))
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	duration := time.Since(startTime)

	// Check for errors
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to execute snapshotter command: %w", err)
		}
	}

	// Get the raw output
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	// Apply selector if provided
	if selector, ok := params["selector"].(string); ok && selector != "" {
		zap.L().Debug("applying selector to command output",
			zap.String("selector", selector))
		selectedOutput, err := util.ApplySelector(stdoutStr, selector)
		if err == nil {
			// Selector applied successfully
			stdoutStr = selectedOutput
			zap.L().Debug("applied selector to command output",
				zap.String("selector", selector))
		} else {
			return nil, errors.Wrap(err, "failed to apply selector to command output")
		}
	}

	// Return the result
	result := &ExecuteResult{
		ExitCode: exitCode,
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
		Duration: duration,
	}

	return result, nil
}

// CreateSnapshot creates a CommandSnapshot from a command execution
func (e *SnapshotExecutor) CreateSnapshot(cmdStr string, result *ExecuteResult, envName string, suiteName string, caseIndex string, caseName string) *snapshot.CommandSnapshot {
	id := snapshot.MakeSnapshotID(suiteName, envName, caseIndex, caseName)

	output := snapshot.CommandOutput{
		Command:     cmdStr,
		ExitCode:    result.ExitCode,
		Stdout:      result.Stdout,
		Stderr:      result.Stderr,
		Environment: envName,
		Duration:    result.Duration.String(),
	}

	zap.L().Debug("creating snapshot from snapshotter command",
		zap.String("id", id),
		zap.String("suite", suiteName),
		zap.String("env", envName),
		zap.String("case_index", caseIndex),
		zap.String("case_name", caseName))

	return &snapshot.CommandSnapshot{
		Id:      id,
		Version: 1, // Initial version
		Output:  output,
	}
}
