// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"fmt"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"go.uber.org/zap"
)

// ExecutorFactory creates command executors based on type
type ExecutorFactory struct {
	config *config.Config
}

// NewExecutorFactory creates a new executor factory
func NewExecutorFactory(cfg *config.Config) *ExecutorFactory {
	return &ExecutorFactory{
		config: cfg,
	}
}

// GetExecutor creates and returns the appropriate executor for the given command type and environment
func (f *ExecutorFactory) GetExecutor(cmdType CommandType, env config.Environments) (CommandExecutor, error) {
	zap.L().Debug("Creating executor",
		zap.String("type", string(cmdType)),
		zap.String("environment", env.Name))

	switch cmdType {
	case RoverCtlCommandType:
		return NewRoverCtlExecutor(f.config.RoverCtl, env), nil
	case SnapshotCommandType:
		return NewSnapshotExecutor(f.config.Snapshotter, env), nil
	default:
		return nil, fmt.Errorf("unsupported command type: %s", cmdType)
	}
}

// GetCommandType determines the command type based on the case configuration
// If type is not specified, defaults to RoverCtlCommandType
func GetCommandType(c config.Case) CommandType {
	if c.Type == "" {
		return RoverCtlCommandType
	}
	return CommandType(c.Type)
}