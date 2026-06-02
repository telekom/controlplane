// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1
// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
)

var _ = Describe("LintingConfig", func() {
	Describe("IsBasepathWhitelisted", func() {
		It("should return false when WhitelistedBasepaths is empty", func() {
			cfg := &apiv1.LintingConfig{}
			Expect(cfg.IsBasepathWhitelisted("/eni/test/v1")).To(BeFalse())
		})

		It("should return true for exact match", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/eni/test/v1"},
			}
			Expect(cfg.IsBasepathWhitelisted("/eni/test/v1")).To(BeTrue())
		})

		It("should match case-insensitively", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/ENI/Test/v1"},
			}
			Expect(cfg.IsBasepathWhitelisted("/eni/test/v1")).To(BeTrue())
		})

		It("should return false for non-matching basepath", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/other/path/v1"},
			}
			Expect(cfg.IsBasepathWhitelisted("/eni/test/v1")).To(BeFalse())
		})

		It("should check all entries", func() {
			cfg := &apiv1.LintingConfig{
				WhitelistedBasepaths: []string{"/first/v1", "/second/v2", "/eni/test/v1"},
			}
			Expect(cfg.IsBasepathWhitelisted("/eni/test/v1")).To(BeTrue())
		})
	})
})
