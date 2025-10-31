// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"context"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/snapshot"
)

// CommandExecutor defines an interface for different command execution types
type CommandExecutor interface {
	// Execute runs a command and returns its output
	Execute(ctx context.Context, cmdStr string, params map[string]interface{}) (*ExecuteResult, error)

	// CreateSnapshot creates a CommandSnapshot from a command execution
	CreateSnapshot(cmdStr string, result *ExecuteResult, envName string, suiteName string, caseIndex string, caseName string) *snapshot.CommandSnapshot
}

// CommandType defines the supported command executor types
type CommandType string

const (
	// RoverCtlCommandType is the default command type for executing rover-ctl commands
	RoverCtlCommandType CommandType = "roverctl"

	// SnapshotCommandType is used for executing snapshot commands
	SnapshotCommandType CommandType = "snapshot"
)