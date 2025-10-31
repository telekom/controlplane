// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"fmt"
	"os"
	"strings"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/command"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"go.uber.org/zap"
)

type EnvironmentManager interface {
	GetEnvironment(name string) (*config.Environments, error)
	GetAllEnvironments() []config.Environments
	ResolveTokenFromEnv() error
	SetupTestEnvironment(environment string) (*config.Environments, error)
	GetExecutor(envName string) (*command.Executor, error)
	ValidateEnvironments(envNames []string) error
	CollectEnvironmentNames(suites []config.Suite) []string
}

var _ EnvironmentManager = (*Manager)(nil)

// Manager handles environment setup for tests
type Manager struct {
	environments   []config.Environments
	executors      map[string]*command.Executor
	roverCtlConfig config.RoverCtlConfig
}

// NewManager creates a new environment manager
func NewManager(environments []config.Environments, roverCtlConfig config.RoverCtlConfig) *Manager {
	return &Manager{
		environments:   environments,
		executors:      make(map[string]*command.Executor),
		roverCtlConfig: roverCtlConfig,
	}
}

// GetEnvironment returns an environment by name
func (m *Manager) GetEnvironment(name string) (*config.Environments, error) {
	for _, env := range m.environments {
		if env.Name == name {
			return &env, nil
		}
	}
	return nil, fmt.Errorf("environment not found: %s", name)
}

// GetAllEnvironments returns all configured environments
func (m *Manager) GetAllEnvironments() []config.Environments {
	return m.environments
}

// ResolveTokenFromEnv resolves token values that reference environment variables
func (m *Manager) ResolveTokenFromEnv() error {
	for i := range m.environments {
		env := &m.environments[i]
		if strings.HasPrefix(env.Token, "env:") {
			envVarName := strings.TrimPrefix(env.Token, "env:")
			envValue := os.Getenv(envVarName)
			if envValue == "" {
				return fmt.Errorf("environment variable not set: %s", envVarName)
			}
			env.Token = envValue
		}
	}
	return nil
}

// SetupTestEnvironment prepares the environment for a test
func (m *Manager) SetupTestEnvironment(environment string) (*config.Environments, error) {
	// Get the environment configuration
	env, err := m.GetEnvironment(environment)
	if err != nil {
		return nil, err
	}

	// Ensure token is resolved
	if strings.HasPrefix(env.Token, "env:") {
		envVarName := strings.TrimPrefix(env.Token, "env:")
		envValue := os.Getenv(envVarName)
		if envValue == "" {
			return nil, fmt.Errorf("environment variable not set: %s", envVarName)
		}
		env.Token = envValue
	}

	return env, nil
}

// GetExecutor returns a cached executor for the specified environment
func (m *Manager) GetExecutor(envName string) (*command.Executor, error) {
	// If no environment is specified, use the first one (default)
	if envName == "" && len(m.environments) > 0 {
		envName = m.environments[0].Name
	}

	// Check the cache first
	if executor, exists := m.executors[envName]; exists {
		return executor, nil
	}

	// Look up the environment
	env, err := m.GetEnvironment(envName)
	if err != nil {
		return nil, fmt.Errorf("environment not found: %s", envName)
	}

	// Create a new executor and cache it
	executor := command.NewExecutor(m.roverCtlConfig, *env)
	m.executors[envName] = executor

	zap.L().Debug("Created new executor for environment",
		zap.String("environment", envName))

	return executor, nil
}

// ValidateEnvironments checks if all environment names in the provided list exist
func (m *Manager) ValidateEnvironments(envNames []string) error {
	for _, name := range envNames {
		if name != "" {
			_, err := m.GetEnvironment(name)
			if err != nil {
				return fmt.Errorf("invalid environment: %s", name)
			}
		}
	}
	return nil
}

// CollectEnvironmentNames gathers all unique environment names from suites and cases
func (m *Manager) CollectEnvironmentNames(suites []config.Suite) []string {
	envSet := make(map[string]struct{})

	// Collect from suites and cases
	for _, suite := range suites {
		// Add all environments from the suite's Environments list
		for _, env := range suite.Environments {
			if env != "" {
				envSet[env] = struct{}{}
			}
		}

		// Add case-specific environments
		for _, c := range suite.Cases {
			if c.Environment != "" {
				envSet[c.Environment] = struct{}{}
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(envSet))
	for env := range envSet {
		result = append(result, env)
	}

	return result
}
