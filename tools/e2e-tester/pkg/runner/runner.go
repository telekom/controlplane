// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/environment"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/report"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/snapshot"
	"go.uber.org/zap"
)

// Runner is the main test orchestrator
type Runner struct {
	config         *config.Config
	envManager     environment.EnvironmentManager
	reporter       report.Reporter
	snapshotMgr    snapshot.SnapshotManager
	updateMode     bool
	snapshotsDir   string
	continueOnFail bool
	suiteFilter    string // Only run the specified suite (optional)
	envFilter      string // Only run in the specified environment (optional)
}

// RunnerOptions contains options for the runner
type RunnerOptions struct {
	UpdateMode     bool
	ContinueOnFail bool
	SnapshotsDir   string
	Reporter       report.Reporter // Custom reporter (optional)
	SuiteFilter    string          // Only run the specified suite (optional)
	EnvFilter      string          // Only run in the specified environment (optional)
}

// buildSuites constructs SuiteRunners for all suites and environments
func (r *Runner) buildSuites() ([]*SuiteRunner, error) {
	var suiteRunners []*SuiteRunner

	// Validate config
	if r.config == nil {
		return nil, fmt.Errorf("configuration is nil")
	}

	// Check if we have any suites
	if len(r.config.Suites) == 0 {
		return nil, fmt.Errorf("no test suites defined in configuration")
	}

	// Log filtering information
	if r.suiteFilter != "" {
		zap.L().Info("Filtering by suite name",
			zap.String("filter", r.suiteFilter))
	}
	if r.envFilter != "" {
		zap.L().Info("Filtering by environment name",
			zap.String("filter", r.envFilter))
	}

	// Apply filters if specified
	filteredSuites := make([]config.Suite, 0, len(r.config.Suites))
	for _, s := range r.config.Suites {
		// Apply suite filter if specified
		if r.suiteFilter != "" && s.Name != r.suiteFilter {
			zap.L().Debug("Skipping suite due to name filter",
				zap.String("suite", s.Name),
				zap.String("filter", r.suiteFilter))
			continue
		}

		// If an environment filter is specified, check if this suite uses that environment
		if r.envFilter != "" {
			// Check suite-level environments
			envMatch := false
			for _, env := range s.Environments {
				if env == r.envFilter {
					envMatch = true
					break
				}
			}

			// If no match at suite level, check case-level environments
			if !envMatch {
				for _, c := range s.Cases {
					if c.Environment == r.envFilter {
						envMatch = true
						break
					}
				}
			}

			// Skip this suite if it doesn't match the environment filter
			if !envMatch {
				zap.L().Debug("Skipping suite due to environment filter",
					zap.String("suite", s.Name),
					zap.String("filter", r.envFilter))
				continue
			}
		}

		// Add this suite to the filtered list
		filteredSuites = append(filteredSuites, s)
	}

	// Check if we have any suites after filtering
	if len(filteredSuites) == 0 {
		// Provide helpful error message based on filters
		if r.suiteFilter != "" && r.envFilter != "" {
			return nil, fmt.Errorf("no suites found matching filter suite='%s', environment='%s'", r.suiteFilter, r.envFilter)
		} else if r.suiteFilter != "" {
			return nil, fmt.Errorf("no suites found matching name filter: '%s'", r.suiteFilter)
		} else if r.envFilter != "" {
			return nil, fmt.Errorf("no suites found using environment: '%s'", r.envFilter)
		} else {
			return nil, fmt.Errorf("no suites found after applying filters")
		}
	}

	// Process each filtered suite
	for idx, suite := range filteredSuites {
		// Create a copy to avoid modifying the original
		suiteCopy := suite

		zap.L().Debug("Processing suite",
			zap.String("suite", suite.Name),
			zap.Int("index", idx+1),
			zap.Int("total", len(filteredSuites)))

		// Process suite environments, which may return multiple suite copies (one per environment)
		processedSuites, err := setupSuiteEnvironments(&suiteCopy)
		if err != nil {
			zap.L().Error("Failed to set up suite environments",
				zap.String("suite", suite.Name),
				zap.Error(err))
			return nil, fmt.Errorf("failed to set up environments for suite '%s': %w", suite.Name, err)
		}

		// Create a SuiteRunner for each processed suite
		for _, s := range processedSuites {

			// Create a new SuiteRunner
			sr := NewSuiteRunner(
				s,
				r.snapshotMgr,
				r.reporter,
				r.updateMode,
				r.continueOnFail,
				r.snapshotsDir,
				r.envManager,
				r.config.RoverCtl,
				r.config, // Pass full config
			)

			if sr == nil {
				return nil, fmt.Errorf("failed to create SuiteRunner for suite %q", suite.Name)
			}

			suiteRunners = append(suiteRunners, sr)

		}
	}

	zap.L().Info("Built suite runners",
		zap.Int("count", len(suiteRunners)))

	return suiteRunners, nil
}

// NewRunner creates a new test runner
func NewRunner(cfg *config.Config, opts RunnerOptions) *Runner {
	// Set default reporter if not provided
	reporter := opts.Reporter
	if reporter == nil {
		panic("reporter cannot be nil")
	}

	// Create snapshot manager
	snapshotsDir := opts.SnapshotsDir
	if snapshotsDir == "" {
		snapshotsDir = "snapshots" // Default snapshots directory
	}
	snapshotMgr := snapshot.NewManager(snapshotsDir)

	// Create environment manager
	envManager := environment.NewManager(cfg.Environments, cfg.RoverCtl)

	// Create and return the runner
	return &Runner{
		config:         cfg,
		envManager:     envManager,
		reporter:       reporter,
		snapshotMgr:    snapshotMgr,
		updateMode:     opts.UpdateMode,
		snapshotsDir:   snapshotsDir,
		continueOnFail: opts.ContinueOnFail,
		suiteFilter:    opts.SuiteFilter,
		envFilter:      opts.EnvFilter,
	}
}

// Run executes test suites for specified environments
// It will orchestrate the execution of test suites based on the provided configuration and options.
// It returns a final report summarizing the test results.
func (r *Runner) Run(ctx context.Context) (*report.FinalReport, error) {
	startTime := time.Now()

	// Check for nil context
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	// Validate runner components
	if r.config == nil {
		return nil, fmt.Errorf("runner configuration cannot be nil")
	}

	if r.envManager == nil {
		return nil, fmt.Errorf("environment manager cannot be nil")
	}

	if r.snapshotMgr == nil {
		return nil, fmt.Errorf("snapshot manager cannot be nil")
	}

	if r.reporter == nil {
		return nil, fmt.Errorf("reporter cannot be nil")
	}

	// Log test run start
	zap.L().Info("Starting test run",
		zap.Bool("update_mode", r.updateMode),
		zap.Bool("continue_on_fail", r.continueOnFail),
		zap.Bool("verbose", r.config.Verbose))

	// Validate the configuration
	if err := r.config.Validate(); err != nil {
		zap.L().Error("Invalid configuration",
			zap.Error(err))
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Resolve environment tokens (e.g., from environment variables)
	if err := r.envManager.ResolveTokenFromEnv(); err != nil {
		zap.L().Error("Failed to resolve environment tokens",
			zap.Error(err))
		return nil, fmt.Errorf("failed to resolve environment tokens: %w", err)
	}

	// Build all suite runners
	zap.L().Debug("Building suite runners")
	suiteRunners, err := r.buildSuites()
	if err != nil {
		zap.L().Error("Failed to build suites",
			zap.Error(err))
		return nil, fmt.Errorf("failed to build suites: %w", err)
	}

	// Check if we have any suites to run after filtering
	if len(suiteRunners) == 0 {
		zap.L().Error("No suites found to run")
		return nil, fmt.Errorf("no suites found to run")
	}

	// Execute all suites and collect results
	zap.L().Info("Executing test suites",
		zap.Int("count", len(suiteRunners)))

	var suiteResults []*report.SuiteResult
	var criticalFailures int

	// Execute all suites and collect results
	for i, sr := range suiteRunners {
		// Check if the context has been canceled
		select {
		case <-ctx.Done():
			zap.L().Warn("Test run interrupted by context cancellation",
				zap.Error(ctx.Err()))
			return r.generateFinalReport(suiteResults, startTime), ctx.Err()
		default:
			// Continue execution
		}

		suiteName := sr.suite.GetName()
		zap.L().Info("Executing test suite",
			zap.String("suite", suiteName),
			zap.Int("index", i+1),
			zap.Int("total", len(suiteRunners)))

		// Execute the suite
		result := sr.Run(ctx)
		if result == nil {
			zap.L().Error("Suite runner returned nil result",
				zap.String("suite", suiteName))
			// Create a placeholder result to avoid nil pointer dereferences
			result = &report.SuiteResult{
				Name:      sr.suite.GetName(),
				StartTime: time.Now(),
				EndTime:   time.Now(),
				Cases:     []*report.TestCaseResult{},
			}
		}

		suiteResults = append(suiteResults, result)

		// Report the suite result
		if r.reporter != nil {
			r.reporter.ReportSuiteResult(result)
		}

		// Check if we should continue on failure
		if !r.continueOnFail {
			// Check if any must-pass test cases failed with error status
			hasCriticalFailure := false
			for _, tc := range result.Cases {
				if tc.MustPass && tc.Status == report.StatusError {
					hasCriticalFailure = true
					criticalFailures++
					zap.L().Warn("Critical test case failed",
						zap.String("suite", result.Name),
						zap.String("case", tc.Name),
						zap.String("environment", tc.Environment))
					break
				}
			}

			// Abort if a critical test failed
			if hasCriticalFailure {
				zap.L().Warn("Aborting test run due to critical failure",
					zap.String("suite", result.Name),
					zap.Int("remaining", len(suiteRunners)-i-1))
				break
			}
		}
	}

	// Generate the final report
	zap.L().Debug("Generating final report",
		zap.Int("suite_count", len(suiteResults)))

	finalReport := r.generateFinalReport(suiteResults, startTime)

	// Log summary
	totalTests := finalReport.TotalPassed + finalReport.TotalFailed + finalReport.TotalSkipped + finalReport.TotalErrors
	zap.L().Info("Test run summary",
		zap.Int("total", totalTests),
		zap.Int("passed", finalReport.TotalPassed),
		zap.Int("failed", finalReport.TotalFailed),
		zap.Int("skipped", finalReport.TotalSkipped),
		zap.Int("errors", finalReport.TotalErrors),
		zap.Int("critical_failures", criticalFailures),
		zap.Float64("duration_sec", finalReport.TotalDuration.Seconds()))

	// Report the final results
	if r.reporter != nil {
		r.reporter.ReportFinal(finalReport)
	}

	// Return the final report
	return finalReport, nil
}

// generateFinalReport generates a final report from all suite results
func (r *Runner) generateFinalReport(suiteResults []*report.SuiteResult, startTime time.Time) *report.FinalReport {
	return report.CalculateFinalReport(suiteResults, startTime, time.Now())
}
