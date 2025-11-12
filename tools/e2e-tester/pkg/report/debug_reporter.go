// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// DebugReporter is a simple reporter that helps debug issues
type DebugReporter struct {
	outDir string
}

// NewDebugReporter creates a new debug reporter
func NewDebugReporter(outDir string) Reporter {
	// Create output directory
	if err := os.MkdirAll(outDir, 0755); err != nil {
		zap.L().Error("Failed to create debug output directory", zap.Error(err))
	}

	return &DebugReporter{
		outDir: outDir,
	}
}

// ReportTestCase reports a test case result
func (r *DebugReporter) ReportTestCase(result *TestCaseResult) {
	// Marshal the test result to JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		zap.L().Error("Failed to marshal test result", zap.Error(err))
		return
	}

	// Write it to a file
	filename := fmt.Sprintf("test-%s-%s.json", result.Environment, result.Name)
	filepath := filepath.Join(r.outDir, filename)

	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		zap.L().Error("Failed to write debug test result", zap.Error(err))
	} else {
		zap.L().Debug("Wrote debug test result", zap.String("file", filepath))
	}
}

// ReportSuiteResult reports a test suite result
func (r *DebugReporter) ReportSuiteResult(result *SuiteResult) {
	// Marshal the suite result to JSON
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		zap.L().Error("Failed to marshal suite result", zap.Error(err))
		return
	}

	// Write it to a file
	filename := fmt.Sprintf("suite-%s-%s.json", result.Environment, result.Name)
	filepath := filepath.Join(r.outDir, filename)

	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		zap.L().Error("Failed to write debug suite result", zap.Error(err))
	} else {
		zap.L().Debug("Wrote debug suite result", zap.String("file", filepath))
	}
}

// ReportFinal generates the final report
func (r *DebugReporter) ReportFinal(report *FinalReport) {
	// Marshal the final report to JSON
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		zap.L().Error("Failed to marshal final report", zap.Error(err))
		return
	}

	// Write it to a file
	filename := fmt.Sprintf("final-report-%s.json", time.Now().Format("20060102-150405"))
	filepath := filepath.Join(r.outDir, filename)

	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		zap.L().Error("Failed to write debug final report", zap.Error(err))
	} else {
		zap.L().Debug("Wrote debug final report", zap.String("file", filepath))
	}
}
