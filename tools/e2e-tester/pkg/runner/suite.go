// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/command"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/environment"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/report"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/snapshot"
	"go.uber.org/zap"
)

// SuiteRunner handles the execution of a test suite
type SuiteRunner struct {
	suite          *config.Suite
	snapshotMgr    snapshot.SnapshotManager
	reporter       report.Reporter
	updateMode     bool
	continueOnFail bool
	baseDir        string
	envManager     environment.EnvironmentManager
	config         *config.Config // Full configuration
	roverCtlConfig config.RoverCtlConfig
	executors      map[string]command.CommandExecutor
}

// NewSuiteRunner creates a new suite runner
func NewSuiteRunner(
	suite *config.Suite,
	snapshotMgr snapshot.SnapshotManager,
	reporter report.Reporter,
	updateMode bool,
	continueOnFail bool,
	baseDir string,
	envManager environment.EnvironmentManager,
	roverCtlConfig config.RoverCtlConfig,
	cfg *config.Config, // Added full config parameter
) *SuiteRunner {
	return &SuiteRunner{
		suite:          suite,
		snapshotMgr:    snapshotMgr,
		reporter:       reporter,
		updateMode:     updateMode,
		continueOnFail: continueOnFail,
		baseDir:        baseDir,
		envManager:     envManager,
		roverCtlConfig: roverCtlConfig,
		config:         cfg,
		executors:      make(map[string]command.CommandExecutor),
	}
}

func setupSuiteEnvironments(suite *config.Suite) ([]*config.Suite, error) {
	// Validate suite pointer
	if suite == nil {
		return nil, fmt.Errorf("suite cannot be nil")
	}

	// Validate suite name
	if suite.Name == "" {
		return nil, fmt.Errorf("suite name cannot be empty")
	}

	// Ensure we have at least one test case
	if len(suite.Cases) == 0 {
		return nil, fmt.Errorf("suite '%s' must have at least one test case", suite.Name)
	}

	// Check for nil test cases
	for i, testCase := range suite.Cases {
		if testCase == nil {
			return nil, fmt.Errorf("suite '%s' has a nil test case at index %d", suite.Name, i)
		}
		if testCase.Name == "" {
			return nil, fmt.Errorf("suite '%s' has a test case with empty name at index %d", suite.Name, i)
		}
	}

	// Check environment configuration patterns
	hasCaseEnvConfig := slices.ContainsFunc(suite.Cases, func(c *config.Case) bool {
		return c.Environment != ""
	})

	envSet := map[string]bool{}
	hasSuiteEnv := len(suite.Environments) > 0
	hasMultipleSuiteEnvs := len(suite.Environments) > 1

	// Validate environment configuration patterns
	if !hasSuiteEnv && !hasCaseEnvConfig {
		return nil, fmt.Errorf("suite '%s' has no environments defined at either suite or case level", suite.Name)
	}

	// Enforce clean environment configuration (either suite-level or case-level, not both)
	if hasCaseEnvConfig && hasMultipleSuiteEnvs {
		return nil, fmt.Errorf("suite '%s' has multiple environments defined but some test cases also define specific environments. Please either define environments at the suite level or at the case level, not both", suite.Name)
	}

	// Ensure all test cases have an environment
	for _, testCase := range suite.Cases {
		if testCase.Environment == "" && !hasSuiteEnv {
			return nil, fmt.Errorf("test case '%s' in suite '%s' does not have an environment defined, and the suite also does not define any environments", testCase.Name, suite.Name)
		}
		if testCase.Environment == "" && hasSuiteEnv {
			testCase.Environment = suite.Environments[0]
		} else {
			envSet[testCase.Environment] = true
		}
	}

	if !hasSuiteEnv {
		if len(envSet) == 0 {
			return nil, fmt.Errorf("suite '%s' has no environments defined at either suite or case level", suite.Name)
		}
		if len(envSet) == 1 {
			suite.Environments = []string{suite.Cases[0].Environment}
		}
		if len(envSet) > 1 {
			suite.Environments = []string{strings.Join(slices.Collect(maps.Keys(envSet)), ", ")}
		}
	}

	// Create the suite copies
	var suites []*config.Suite
	if hasMultipleSuiteEnvs {
		zap.L().Debug("Creating multiple suite copies for environments",
			zap.String("suite", suite.Name),
			zap.Strings("environments", suite.Environments),
			zap.Int("env_count", len(suite.Environments)))

		for _, env := range suite.Environments {
			// Validate environment name
			if env == "" {
				return nil, fmt.Errorf("suite '%s' has an empty environment name", suite.Name)
			}

			// Create a deep copy of the suite for this environment
			newSuite := suite.DeepCopy()
			if newSuite == nil {
				return nil, fmt.Errorf("failed to create deep copy of suite '%s'", suite.Name)
			}

			// Update all test cases to use this environment
			for _, testCase := range newSuite.Cases {
				testCase.Environment = env
			}

			// Update the environment list to only include this environment
			newSuite.Environments = []string{env}
			suites = append(suites, newSuite)
		}

		zap.L().Debug("Created multiple suite copies",
			zap.String("suite", suite.Name),
			zap.Int("copy_count", len(suites)))

		return suites, nil
	}

	// Single environment case - just return the original suite
	zap.L().Debug("Using single suite",
		zap.String("suite", suite.Name),
		zap.Strings("environments", suite.Environments))

	return append(suites, suite), nil
}

// getCommandExecutor returns a cached command executor for the specified environment and command type
func (r *SuiteRunner) getCommandExecutor(envName string, commandType command.CommandType) (command.CommandExecutor, error) {
	// Validate environment name
	if envName == "" {
		return nil, fmt.Errorf("environment name cannot be empty")
	}

	// Generate cache key based on environment and command type
	cacheKey := fmt.Sprintf("%s:%s", envName, commandType)

	// Check if we already have a cached executor
	if executor, exists := r.executors[cacheKey]; exists {
		zap.L().Debug("Using cached executor for environment and type",
			zap.String("environment", envName),
			zap.String("type", string(commandType)))
		return executor, nil
	}

	// Look up the environment
	env, err := r.envManager.GetEnvironment(envName)
	if err != nil {
		zap.L().Error("Failed to get environment",
			zap.String("environment", envName),
			zap.Error(err))
		return nil, fmt.Errorf("environment not found: %s: %w", envName, err)
	}

	// Validate environment configuration
	if env.Name == "" {
		return nil, fmt.Errorf("invalid environment configuration: empty name")
	}

	var executor command.CommandExecutor

	// Create the appropriate executor based on the command type
	switch commandType {
	case command.SnapshotCommandType:
		executor = command.NewSnapshotExecutor(r.config.Snapshotter, *env)
	case command.RoverCtlCommandType:
		executor = command.NewRoverCtlExecutor(r.roverCtlConfig, *env)
	default:
		return nil, fmt.Errorf("unsupported command type: %s", commandType)
	}

	// Cache the executor
	r.executors[cacheKey] = executor

	zap.L().Debug("Created new executor",
		zap.String("environment", envName),
		zap.String("type", string(commandType)))

	return executor, nil
}

// getExecutor returns a cached rover-ctl executor for the specified environment
// This is kept for backward compatibility
func (r *SuiteRunner) getExecutor(envName string) (*command.RoverCtlExecutor, error) {
	executor, err := r.getCommandExecutor(envName, command.RoverCtlCommandType)
	if err != nil {
		return nil, err
	}

	// Type assertion to get the concrete type
	roverExecutor, ok := executor.(*command.RoverCtlExecutor)
	if !ok {
		return nil, fmt.Errorf("failed to convert executor to rover-ctl executor")
	}

	return roverExecutor, nil
}

// Run executes all cases in the suite
func (r *SuiteRunner) Run(ctx context.Context) *report.SuiteResult {
	startTime := time.Now()

	if len(r.suite.Environments) != 1 {
		panic(fmt.Sprintf("suite must have exactly one environment at this point, found %d", len(r.suite.Environments)))
	}

	// Create a new suite result
	suiteResult := &report.SuiteResult{
		Name:        r.suite.Name,
		Description: r.suite.Description,
		StartTime:   startTime,
		Environment: r.suite.Environments[0],
		Cases:       make([]*report.TestCaseResult, 0, len(r.suite.Cases)),
	}

	zap.L().Info("Starting test suite",
		zap.String("suite", r.suite.Name),
		zap.Int("cases", len(r.suite.Cases)))

	// Execute each test case in order
	for i, testCase := range r.suite.Cases {
		// Execute the test case
		caseResult := r.runCase(ctx, *testCase, i)
		suiteResult.Cases = append(suiteResult.Cases, caseResult)

		r.reporter.ReportTestCase(caseResult)

		// Check if we should continue after a failure
		if !r.continueOnFail && testCase.MustPass && caseResult.Status == report.StatusError {
			zap.L().Warn("Aborting suite execution due to critical test failure",
				zap.String("case", testCase.Name),
				zap.String("suite", r.suite.Name),
			)
			break
		}
	}

	// Record end time
	suiteResult.EndTime = time.Now()

	zap.L().Info("Finished test suite",
		zap.String("suite", r.suite.Name),
		zap.Float64("duration_sec", time.Since(startTime).Seconds()))

	return suiteResult
}

// runCase executes a single test case
func (r *SuiteRunner) runCase(ctx context.Context, c config.Case, caseIndex int) *report.TestCaseResult {
	startTime := time.Now()

	// Create a result object to track the test case execution
	result := &report.TestCaseResult{
		Name:        c.Name,
		Description: c.Description,
		Command:     c.Command,
		Environment: c.Environment,
		MustPass:    c.MustPass,
	}

	// Determine the command type - default to "roverctl" if not specified
	cmdType := command.RoverCtlCommandType
	if c.Type != "" {
		cmdType = command.CommandType(c.Type)
	}

	// Log start of test case execution
	zap.L().Debug("Executing test case",
		zap.String("case", c.Name),
		zap.String("environment", c.Environment),
		zap.String("command", c.Command),
		zap.String("type", string(cmdType)),
		zap.Duration("wait_before", c.WaitBefore),
		zap.Duration("wait_after", c.WaitAfter),
	)

	time.Sleep(c.WaitBefore)
	defer time.Sleep(c.WaitAfter)

	// Get the executor for this environment and command type
	executor, err := r.getCommandExecutor(c.Environment, cmdType)
	if err != nil {
		result.Status = report.StatusError
		result.Error = fmt.Errorf("failed to get executor: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Prepare params for executor
	params := make(map[string]any)
	if c.Params != nil {
		// Copy existing params
		maps.Copy(params, c.Params)
	}

	// Add selector if present
	if c.Selector != "" {
		params["selector"] = c.Selector
		zap.L().Debug("added selector to execution params",
			zap.String("selector", c.Selector))
	}

	// Execute the command
	execResult, err := executor.Execute(ctx, c.Command, params)
	if err != nil {
		result.Status = report.StatusError
		result.Error = fmt.Errorf("command execution failed: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Record execution results
	result.ExitCode = execResult.ExitCode
	result.Duration = execResult.Duration

	// Create a snapshot ID using the suite name, environment, case index, and case name
	caseIndexStr := fmt.Sprintf("%d", caseIndex)

	// Create a snapshot from the execution result
	snapshot := executor.CreateSnapshot(
		c.Command,
		execResult,
		c.Environment,
		r.suite.Name,
		caseIndexStr,
		c.Name,
	)

	// If we're in update mode, store the snapshot and mark the test as passed
	if r.updateMode {
		if err := r.snapshotMgr.StoreSnapshot(ctx, snapshot); err != nil {
			result.Status = report.StatusError
			result.Error = fmt.Errorf("failed to store snapshot: %w", err)
			return result
		}

		// In update mode, we consider the test passed
		result.Status = report.StatusPassed
		return result
	}

	// If we're not in update mode, compare with the expected snapshot
	expectedSnapshot, err := r.snapshotMgr.GetLatestSnapshot(ctx, snapshot.Id)
	if err != nil {
		// If the snapshot doesn't exist, it means this is the first run
		// and it must be created
		if err := r.snapshotMgr.StoreSnapshot(ctx, snapshot); err != nil {
			result.Status = report.StatusError
			result.Error = fmt.Errorf("failed to store initial snapshot: %w", err)
			return result
		}

		result.Status = report.StatusPassed
		return result
	}

	// Compare the snapshots
	comparison := r.snapshotMgr.CompareSnapshots(expectedSnapshot, snapshot)

	// If there are differences, the test failed
	if comparison.HasChanges {
		result.Status = report.StatusFailed
		result.ComparisonDiff = comparison.DiffText
	} else {
		result.Status = report.StatusPassed
	}

	return result
}
