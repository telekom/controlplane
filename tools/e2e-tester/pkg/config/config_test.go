// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RunPolicy", func() {

	DescribeTable("IsValid",
		func(policy RunPolicy, expected bool) {
			Expect(policy.IsValid()).To(Equal(expected))
		},
		Entry("canonical RunOnSuccess", RunPolicyRunOnSuccess, true),
		Entry("canonical FailFast", RunPolicyFailFast, true),
		Entry("canonical Always", RunPolicyAlways, true),
		Entry("lowercase runonsuccess", RunPolicy("runonsuccess"), true),
		Entry("uppercase FAILFAST", RunPolicy("FAILFAST"), true),
		Entry("mixed case aLwAyS", RunPolicy("aLwAyS"), true),
		Entry("invalid policy", RunPolicy("invalid"), false),
		Entry("empty policy", RunPolicy(""), false),
		Entry("old normal value", RunPolicy("normal"), false),
		Entry("old critical value", RunPolicy("critical"), false),
	)

	DescribeTable("Normalize",
		func(policy RunPolicy, expected RunPolicy) {
			Expect(policy.Normalize()).To(Equal(expected))
		},
		Entry("canonical RunOnSuccess", RunPolicyRunOnSuccess, RunPolicyRunOnSuccess),
		Entry("canonical FailFast", RunPolicyFailFast, RunPolicyFailFast),
		Entry("canonical Always", RunPolicyAlways, RunPolicyAlways),
		Entry("lowercase runonsuccess", RunPolicy("runonsuccess"), RunPolicyRunOnSuccess),
		Entry("uppercase FAILFAST", RunPolicy("FAILFAST"), RunPolicyFailFast),
		Entry("mixed case aLwAyS", RunPolicy("aLwAyS"), RunPolicyAlways),
		Entry("invalid returns unchanged", RunPolicy("invalid"), RunPolicy("invalid")),
		Entry("empty returns unchanged", RunPolicy(""), RunPolicy("")),
	)
})

var _ = Describe("Case", func() {

	DescribeTable("GetRunPolicy",
		func(c Case, expected RunPolicy) {
			Expect(c.GetRunPolicy()).To(Equal(expected))
		},
		Entry("empty defaults to RunOnSuccess", Case{}, RunPolicyRunOnSuccess),
		Entry("explicit RunOnSuccess", Case{RunPolicy: RunPolicyRunOnSuccess}, RunPolicyRunOnSuccess),
		Entry("explicit FailFast", Case{RunPolicy: RunPolicyFailFast}, RunPolicyFailFast),
		Entry("explicit Always", Case{RunPolicy: RunPolicyAlways}, RunPolicyAlways),
		Entry("normalizes lowercase failfast", Case{RunPolicy: "failfast"}, RunPolicyFailFast),
		Entry("normalizes mixed case always", Case{RunPolicy: "aLwAyS"}, RunPolicyAlways),
	)

	DescribeTable("IsCritical",
		func(c Case, expected bool) {
			Expect(c.IsCritical()).To(Equal(expected))
		},
		Entry("FailFast is critical", Case{RunPolicy: RunPolicyFailFast}, true),
		Entry("lowercase failfast is critical", Case{RunPolicy: "failfast"}, true),
		Entry("RunOnSuccess is not critical", Case{RunPolicy: RunPolicyRunOnSuccess}, false),
		Entry("Always is not critical", Case{RunPolicy: RunPolicyAlways}, false),
		Entry("empty is not critical", Case{}, false),
	)

	DescribeTable("ShouldAlwaysRun",
		func(c Case, expected bool) {
			Expect(c.ShouldAlwaysRun()).To(Equal(expected))
		},
		Entry("Always should always run", Case{RunPolicy: RunPolicyAlways}, true),
		Entry("lowercase always should always run", Case{RunPolicy: "always"}, true),
		Entry("FailFast should not always run", Case{RunPolicy: RunPolicyFailFast}, false),
		Entry("RunOnSuccess should not always run", Case{RunPolicy: RunPolicyRunOnSuccess}, false),
		Entry("empty should not always run", Case{}, false),
	)
})

var _ = Describe("Suite", func() {

	DescribeTable("GetName",
		func(suite Suite, expected string) {
			Expect(suite.GetName()).To(Equal(expected))
		},
		Entry("no environments", Suite{Name: "my-suite"}, "my-suite"),
		Entry("single environment", Suite{Name: "my-suite", Environments: []string{"prod"}}, "my-suite [prod]"),
		Entry("multiple environments", Suite{Name: "my-suite", Environments: []string{"dev", "staging"}}, "my-suite [dev_staging]"),
	)
})

var _ = Describe("Validation", func() {

	Context("case-insensitive run policies", func() {
		DescribeTable("should accept",
			func(policy RunPolicy) {
				cfg := &Config{
					Snapshotter:  SnapshotterConfig{Binary: "snapshotter"},
					RoverCtl:     RoverCtlConfig{Binary: "roverctl"},
					Environments: []Environments{{Name: "test-env", Token: "test-token"}},
					Suites: []Suite{{
						Name:  "test-suite",
						Cases: []*Case{{Name: "test-case", Command: "--version", RunPolicy: policy}},
					}},
				}
				Expect(cfg.Validate()).To(Succeed())
			},
			Entry("runonsuccess", RunPolicy("runonsuccess")),
			Entry("FAILFAST", RunPolicy("FAILFAST")),
			Entry("aLwAyS", RunPolicy("aLwAyS")),
		)
	})

	It("should reject suite with no cases and no filepath", func() {
		cfg := &Config{
			Snapshotter:  SnapshotterConfig{Binary: "snapshotter"},
			RoverCtl:     RoverCtlConfig{Binary: "roverctl"},
			Environments: []Environments{{Name: "test-env", Token: "test-token"}},
			Suites:       []Suite{{Name: "empty-suite"}},
		}
		Expect(cfg.Validate()).To(HaveOccurred())
	})
})
