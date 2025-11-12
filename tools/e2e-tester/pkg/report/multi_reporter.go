// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package report

// MultiReporter sends reports to multiple reporters
type MultiReporter struct {
	reporters []Reporter
}

// NewMultiReporter creates a new multi-reporter
func NewMultiReporter(reporters ...Reporter) Reporter {
	return &MultiReporter{
		reporters: reporters,
	}
}

// ReportTestCase reports a test case result to all reporters
func (r *MultiReporter) ReportTestCase(result *TestCaseResult) {
	for _, reporter := range r.reporters {
		reporter.ReportTestCase(result)
	}
}

// ReportSuiteResult reports a test suite result to all reporters
func (r *MultiReporter) ReportSuiteResult(result *SuiteResult) {
	for _, reporter := range r.reporters {
		reporter.ReportSuiteResult(result)
	}
}

// ReportFinal generates the final report with all reporters
func (r *MultiReporter) ReportFinal(report *FinalReport) {
	for _, reporter := range r.reporters {
		reporter.ReportFinal(report)
	}
}
