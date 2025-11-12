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

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/snapshot"
	"go.uber.org/zap"
)

// ExecuteResult contains the result of a command execution
type ExecuteResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// RoverCtlExecutor handles executing rover-ctl commands and capturing their outputs
type RoverCtlExecutor struct {
	roverCtlConfig config.RoverCtlConfig
	environment    config.Environments
}

// NewRoverCtlExecutor creates a new rover-ctl command executor
func NewRoverCtlExecutor(roverCtlConfig config.RoverCtlConfig, environment config.Environments) *RoverCtlExecutor {
	return &RoverCtlExecutor{
		roverCtlConfig: roverCtlConfig,
		environment:    environment,
	}
}

// Execute runs a rover-ctl command and returns its output
func (e *RoverCtlExecutor) Execute(ctx context.Context, cmdStr string, params map[string]interface{}) (*ExecuteResult, error) {
	startTime := time.Now()

	// Prepare the command
	fullCmd := fmt.Sprintf("%s %s", e.roverCtlConfig.Binary, cmdStr)
	cmd := exec.CommandContext(ctx, "sh", "-c", fullCmd)

	// Log the full command for debugging
	zap.L().Debug("executing roverctl command",
		zap.String("full_command", fullCmd),
		zap.String("environment", e.environment.Name))

	// Set up environment variables
	if e.environment.Token != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("ROVER_TOKEN=%s", e.environment.Token))
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
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	// Return the result
	result := &ExecuteResult{
		ExitCode: exitCode,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: duration,
	}

	return result, nil
}

// CreateSnapshot creates a CommandSnapshot from a command execution
func (e *RoverCtlExecutor) CreateSnapshot(cmdStr string, result *ExecuteResult, envName string, suiteName string, caseIndex string, caseName string) *snapshot.CommandSnapshot {

	id := snapshot.MakeSnapshotID(suiteName, envName, caseIndex, caseName)

	output := snapshot.CommandOutput{
		Command:     cmdStr,
		ExitCode:    result.ExitCode,
		Stdout:      result.Stdout,
		Stderr:      result.Stderr,
		Environment: envName,
		Duration:    result.Duration.String(),
	}

	zap.L().Debug("creating new snapshot",
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

// For backward compatibility
// Executor is the old name for RoverCtlExecutor
type Executor = RoverCtlExecutor

// NewExecutor creates a new command executor (for backward compatibility)
func NewExecutor(roverCtlConfig config.RoverCtlConfig, environment config.Environments) *Executor {
	return NewRoverCtlExecutor(roverCtlConfig, environment)
}
