// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fatih/color"
	"go.uber.org/zap"
)

// TestStatus represents the status of a test case
type TestStatus string

const (
	StatusPassed  TestStatus = "PASSED"
	StatusFailed  TestStatus = "FAILED"
	StatusSkipped TestStatus = "SKIPPED"
	StatusError   TestStatus = "ERROR"
)

// TestCaseResult represents the result of a test case
type TestCaseResult struct {
	Name           string
	Description    string // Optional description of the test case purpose
	Command        string
	Status         TestStatus
	ExitCode       int
	Duration       time.Duration
	Error          error
	ComparisonDiff string
	Environment    string
	MustPass       bool
	SkipReason     string // Reason for skipping this test case
}

// SuiteResult represents the result of a test suite
type SuiteResult struct {
	Name        string
	Description string // Optional description of the test suite purpose
	Environment string
	Cases       []*TestCaseResult
	StartTime   time.Time
	EndTime     time.Time
}

// FinalReport represents the final report of all test suites
type FinalReport struct {
	Suites        []*SuiteResult
	TotalPassed   int
	TotalFailed   int
	TotalSkipped  int
	TotalErrors   int
	TotalDuration time.Duration
	StartTime     time.Time
	EndTime       time.Time
}

// ConsoleReporter generates reports for test executions to the console
type ConsoleReporter struct {
	verbose bool
	output  io.Writer
}

// NewConsoleReporter creates a new console reporter
func NewConsoleReporter(output io.Writer, verbose bool) Reporter {
	return &ConsoleReporter{
		verbose: verbose,
		output:  output,
	}
}

// ReportTestCase reports a single test case result
func (r *ConsoleReporter) ReportTestCase(result *TestCaseResult) {
	// Log the test case result with appropriate level
	fields := []zap.Field{
		zap.String("case", result.Name),
		zap.String("environment", result.Environment),
		zap.Float64("duration_sec", result.Duration.Seconds()),
		zap.String("command", result.Command),
	}

	switch result.Status {
	case StatusPassed:
		zap.L().Debug("Test case passed", fields...)
	case StatusFailed:
		zap.L().Warn("Test case failed", fields...)
	case StatusSkipped:
		zap.L().Info("Test case skipped", fields...)
	case StatusError:
		if result.Error != nil {
			fields = append(fields, zap.Error(result.Error))
		}
		zap.L().Error("Test case error", fields...)
	}

	// Only show test output in non-verbose mode or if test didn't pass
	if !r.verbose && result.Status == StatusPassed {
		return
	}

	// Format output for console with more emphasis on test name for better readability
	fmt.Fprintf(
		r.output,
		"%s Test: %s\n       Environment: %s (%.2fs)",
		getStatusIcon(result.Status),
		color.WhiteString(result.Name),
		result.Environment,
		result.Duration.Seconds(),
	)

	// Display description if available
	if result.Description != "" {
		fmt.Fprintf(r.output, "\n       Description: %s", result.Description)
	}

	if r.verbose || result.Status != StatusPassed {
		fmt.Fprintf(r.output, "  Command: %s\n", result.Command)

		if result.Error != nil {
			fmt.Fprintf(r.output, "  "+color.RedString("Error: %s\n"), result.Error)
		}

		// If the test case has MustPass flag and is in ERROR state, provide additional context
		if result.MustPass && result.Status == StatusError {
			fmt.Fprintln(r.output, "  "+color.RedString("Critical Error:"))
			fmt.Fprintf(r.output, "    %s\n", color.RedString("This test case is marked as must_pass but had an execution error"))
			fmt.Fprintf(r.output, "    %s\n", color.RedString("This will cause the entire test suite to abort"))
		}

		// If the test case has MustPass flag and FAILED (comparison), it doesn't abort but should be highlighted
		if result.MustPass && result.Status == StatusFailed {
			fmt.Fprintln(r.output, "  "+color.YellowString("Important Test:"))
			fmt.Fprintf(r.output, "    %s\n", color.YellowString("This test case is marked as must_pass"))
			fmt.Fprintf(r.output, "    %s\n", color.YellowString("The test will continue but the final result will be marked as failed"))
		}

		// If command failed with non-zero exit code, show exit code
		if result.ExitCode != 0 {
			fmt.Fprintf(r.output, "  "+color.YellowString("Exit Code: %d\n"), result.ExitCode)
		}

		if result.Status == StatusFailed && result.ComparisonDiff != "" {
			fmt.Fprintln(r.output, "\n  "+color.CyanString("📋 Summary of Differences:"))

			// Extract key information from the diff without overwhelming details
			diffLines := strings.Split(result.ComparisonDiff, "\n")

			// Find key differences and format them clearly
			exitCodeDiff := false
			stderrDiff := false
			stdoutDiff := false

			for _, line := range diffLines {
				if strings.Contains(line, "exit_code") {
					exitCodeDiff = true
				} else if strings.Contains(line, "stderr") {
					stderrDiff = true
				} else if strings.Contains(line, "stdout") {
					stdoutDiff = true
				}
			}

			// Print simplified summary of what's different
			if exitCodeDiff {
				fmt.Fprintf(r.output, "    %s %s\n", color.RedString("●"), "Exit code differs from expected")
			}
			if stderrDiff {
				fmt.Fprintf(r.output, "    %s %s\n", color.RedString("●"), "Error output differs from expected")
			}
			if stdoutDiff {
				fmt.Fprintf(r.output, "    %s %s\n", color.RedString("●"), "Standard output differs from expected")
			}

			// Show how to view full diff in verbose mode
			fmt.Fprintf(r.output, "\n  %s\n", color.YellowString("Run with --verbose flag to see detailed diff"))

			// Add hint about updating snapshots
			fmt.Fprintf(r.output, "  %s\n", color.CyanString("To update snapshots: use --update flag"))

			// Only show full diff in verbose mode
			if r.verbose {
				fmt.Fprintln(r.output, "\n  "+color.CyanString("Complete Diff:"))
				fmt.Fprintf(r.output, "%s\n", indent(result.ComparisonDiff, 4))
			}
		}
	}
}

// ReportSuiteResult reports a test suite result
func (r *ConsoleReporter) ReportSuiteResult(result *SuiteResult) {
	var passed, failed, skipped, errors int

	for _, c := range result.Cases {
		switch c.Status {
		case StatusPassed:
			passed++
		case StatusFailed:
			failed++
		case StatusSkipped:
			skipped++
		case StatusError:
			errors++
		}
	}

	duration := result.EndTime.Sub(result.StartTime)

	// Log the suite result
	zap.L().Debug("Test suite completed",
		zap.String("suite", result.Name),
		zap.String("environment", result.Environment),
		zap.Duration("duration", duration),
		zap.Int("total", len(result.Cases)),
		zap.Int("passed", passed),
		zap.Int("failed", failed),
		zap.Int("skipped", skipped),
		zap.Int("errors", errors))

	// Format output for console with improved readability
	fmt.Fprintln(r.output, "\n"+color.CyanString("Test Suite Summary"))
	fmt.Fprintf(r.output, "  %s: %s\n", color.WhiteString("Suite"), result.Name)
	fmt.Fprintf(r.output, "  %s: %s\n", color.WhiteString("Environment"), result.Environment)
	fmt.Fprintf(r.output, "  %s: %.2f seconds\n", color.WhiteString("Duration"), duration.Seconds())

	// Create a visual summary bar for results
	fmt.Fprintln(r.output, "  "+color.WhiteString("Results:"))

	// Show total
	fmt.Fprintf(r.output, "    Total Tests: %d\n", len(result.Cases))

	// Show each result with appropriate formatting
	if passed > 0 {
		fmt.Fprintf(r.output, "    %s %s\n", color.GreenString("✓"), color.GreenString("Passed: %d", passed))
	}
	if failed > 0 {
		fmt.Fprintf(r.output, "    %s %s\n", color.RedString("×"), color.RedString("Failed: %d", failed))
	}
	if errors > 0 {
		fmt.Fprintf(r.output, "    %s %s\n", color.MagentaString("!"), color.MagentaString("Errors: %d", errors))
	}
	if skipped > 0 {
		fmt.Fprintf(r.output, "    %s %s\n", color.YellowString("⸮"), color.YellowString("Skipped: %d", skipped))
	}
}

// ReportFinal generates the final report
func (r *ConsoleReporter) ReportFinal(report *FinalReport) {
	duration := report.EndTime.Sub(report.StartTime)

	// Log final report at the appropriate level
	hasFailed := report.TotalFailed > 0 || report.TotalErrors > 0
	logFields := []zap.Field{
		zap.Duration("duration", duration),
		zap.Int("suites", len(report.Suites)),
		zap.Int("total", report.TotalPassed+report.TotalFailed+report.TotalSkipped+report.TotalErrors),
		zap.Int("passed", report.TotalPassed),
		zap.Int("failed", report.TotalFailed),
		zap.Int("skipped", report.TotalSkipped),
		zap.Int("errors", report.TotalErrors),
	}

	if hasFailed {
		// Collect failed test cases for logging
		var failedTests []string
		for _, suite := range report.Suites {
			for _, c := range suite.Cases {
				if c.Status == StatusFailed || c.Status == StatusError {
					failedTests = append(failedTests, fmt.Sprintf("%s.%s", suite.Name, c.Name))
				}
			}
		}
		logFields = append(logFields, zap.Strings("failed_tests", failedTests))
		zap.L().Warn("Test run completed with failures", logFields...)
	} else {
		zap.L().Debug("Test run completed successfully", logFields...)
	}

	// Format output for console with cleaner, more user-friendly design
	totalTests := report.TotalPassed + report.TotalFailed + report.TotalSkipped + report.TotalErrors

	// Create a visually clear header
	header := color.CyanString("⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯")
	fmt.Fprintln(r.output, "\n"+header)
	fmt.Fprintln(r.output, color.CyanString("           FINAL TEST RESULTS"))
	fmt.Fprintln(r.output, header)

	// Show basic test information
	fmt.Fprintf(r.output, "  %s: %.2f seconds\n", color.WhiteString("Total Duration"), duration.Seconds())
	fmt.Fprintf(r.output, "  %s: %d\n", color.WhiteString("Test Suites"), len(report.Suites))
	fmt.Fprintf(r.output, "  %s: %d\n", color.WhiteString("Total Tests"), totalTests)

	// Show a visual summary with icons for better readability
	fmt.Fprintln(r.output, "\n  "+color.WhiteString("Results Summary:"))

	if report.TotalPassed > 0 {
		passPercent := int((float64(report.TotalPassed) / float64(totalTests)) * 100)
		fmt.Fprintf(r.output, "    %s %s (%d%%)\n",
			color.GreenString("✓"),
			color.GreenString("Passed: %d", report.TotalPassed),
			passPercent)
	}

	if report.TotalFailed > 0 {
		failPercent := int((float64(report.TotalFailed) / float64(totalTests)) * 100)
		fmt.Fprintf(r.output, "    %s %s (%d%%)\n",
			color.RedString("×"),
			color.RedString("Failed: %d", report.TotalFailed),
			failPercent)
	}

	if report.TotalErrors > 0 {
		errorPercent := int((float64(report.TotalErrors) / float64(totalTests)) * 100)
		fmt.Fprintf(r.output, "    %s %s (%d%%)\n",
			color.MagentaString("!"),
			color.MagentaString("Errors: %d", report.TotalErrors),
			errorPercent)
	}

	if report.TotalSkipped > 0 {
		skipPercent := int((float64(report.TotalSkipped) / float64(totalTests)) * 100)
		fmt.Fprintf(r.output, "    %s %s (%d%%)\n",
			color.YellowString("-"),
			color.YellowString("Skipped: %d", report.TotalSkipped),
			skipPercent)
	}

	if hasFailed {
		fmt.Fprintln(r.output, "\n"+color.RedString("─────────────────────────────────"))
		fmt.Fprintln(r.output, color.RedString("  Failed Tests - Quick Summary"))
		fmt.Fprintln(r.output, color.RedString("─────────────────────────────────"))

		testCount := 1
		for _, suite := range report.Suites {
			for _, c := range suite.Cases {
				if c.Status == StatusFailed || c.Status == StatusError {
					// Create a visually clear, concise summary of each failure
					var icon, reason string

					if c.Status == StatusFailed {
						icon = color.RedString("×")
						reason = "snapshot mismatch"
					} else {
						icon = color.MagentaString("!")
						reason = "execution error"
					}

					// Main test information
					fmt.Fprintf(
						r.output,
						"  %s %s: %s\n",
						icon,
						color.WhiteString("Test #%d", testCount),
						color.WhiteString(c.Name),
					)

					// Show reason and location
					fmt.Fprintf(
						r.output,
						"    %s: %s\n    %s: %s.%s\n",
						color.YellowString("Reason"),
						reason,
						color.YellowString("Location"),
						suite.Name,
						c.Environment,
					)

					// Show command in a compact form
					fmt.Fprintf(r.output, "    %s: %s\n",
						color.YellowString("Command"),
						c.Command,
					)

					// Show exit code if non-zero
					if c.ExitCode != 0 {
						fmt.Fprintf(r.output, "    %s: %d\n",
							color.YellowString("Exit Code"),
							c.ExitCode,
						)
					}

					// Show error message if present (only first line to keep it concise)
					if c.Error != nil {
						errorMsg := c.Error.Error()
						// Truncate long errors
						if len(errorMsg) > 80 {
							errorMsg = errorMsg[:77] + "..."
						}
						fmt.Fprintf(r.output, "    %s: %s\n",
							color.YellowString("Error"),
							errorMsg,
						)
					}

					// Add must_pass flag if relevant
					if c.MustPass {
						fmt.Fprintln(r.output, "    "+color.RedString("⚠ Critical Test (must_pass)"))
					}

					// Add a separator between test entries
					if testCount < report.TotalFailed+report.TotalErrors {
						fmt.Fprintln(r.output, "  "+color.YellowString("·················"))
					}

					testCount++
				}
			}
		}

		// Add helpful reminders at the bottom
		fmt.Fprintln(r.output, "\n  "+color.CyanString("Available Options:"))
		fmt.Fprintln(r.output, "    • "+color.CyanString("--verbose")+": Show detailed failure information")
		fmt.Fprintln(r.output, "    • "+color.CyanString("--update")+": Update snapshots with current output")
		fmt.Fprintln(r.output, "    • "+color.CyanString("--continue")+": Continue despite failures")
	}

	// Show final status
	fmt.Fprintln(r.output, "")
	if hasFailed {
		fmt.Fprintln(r.output, color.RedString("  ✘ Some tests failed!"))
		fmt.Fprintln(r.output, color.YellowString("  Run with --verbose flag for detailed output"))
	} else {
		fmt.Fprintln(r.output, color.GreenString("  ✓ All tests passed successfully!"))
	}
	fmt.Fprintln(r.output, color.CyanString("⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯"))
}

// Helper function to indent text
func indent(text string, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	return pad + strings.Replace(text, "\n", "\n"+pad, -1)
}

// Helper function to get status icon
func getStatusIcon(status TestStatus) string {
	switch status {
	case StatusPassed:
		return color.GreenString(" ✓ ")
	case StatusFailed:
		return color.RedString(" × ")
	case StatusSkipped:
		return color.YellowString(" - ")
	case StatusError:
		return color.MagentaString(" ! ")
	default:
		return color.WhiteString(" ? ")
	}
}

// CalculateFinalReport calculates the final report stats from all suite results
func CalculateFinalReport(suites []*SuiteResult, startTime, endTime time.Time) *FinalReport {
	report := &FinalReport{
		Suites:    suites,
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Track which test cases have been counted to avoid counting duplicates
	caseSignatures := make(map[string]bool)

	for _, suite := range suites {
		for _, c := range suite.Cases {
			// Only count cases that run in their intended environment
			// Skip cases that were skipped because they belong to a different environment
			if c.Status == StatusSkipped && c.SkipReason != "" && strings.Contains(c.SkipReason, "meant for environment") {
				// Skip counting this case - it will be counted when run in its proper environment
				continue
			}

			// Create a unique signature for this test case to avoid counting duplicates
			// Use the combination of suite name, case name, and environment
			caseSignature := fmt.Sprintf("%s-%s-%s", suite.Name, c.Name, c.Environment)

			// Only count each test case once
			if _, counted := caseSignatures[caseSignature]; !counted {
				switch c.Status {
				case StatusPassed:
					report.TotalPassed++
				case StatusFailed:
					report.TotalFailed++
				case StatusSkipped:
					report.TotalSkipped++
				case StatusError:
					report.TotalErrors++
				}
				caseSignatures[caseSignature] = true
			}
		}
		report.TotalDuration += suite.EndTime.Sub(suite.StartTime)
	}

	return report
}
