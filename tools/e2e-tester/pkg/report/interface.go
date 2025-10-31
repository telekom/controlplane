// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package report

// Reporter defines the interface for test result reporting
type Reporter interface {
	// ReportTestCase reports a single test case result
	ReportTestCase(result *TestCaseResult)

	// ReportSuiteResult reports a test suite result
	ReportSuiteResult(result *SuiteResult)

	// ReportFinal generates the final report
	ReportFinal(report *FinalReport)
}
