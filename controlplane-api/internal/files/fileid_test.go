// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/controlplane-api/internal/files"
)

var _ = Describe("AC-4: ValidateFileID", func() {
	DescribeTable("accepts", func(id string) {
		Expect(files.ValidateFileID(id)).To(Succeed())
	},
		Entry("canonical lowercase UUID", "0190b2a4-7f3c-7a52-8c1e-3d4f5a6b7c8d"),
		Entry("legacy id", "env--group--team--name.yaml"),
		Entry("legacy id with more parts", "env--group--team--name--v1.yaml"),
	)

	DescribeTable("rejects", func(id string) {
		Expect(files.ValidateFileID(id)).To(HaveOccurred())
	},
		Entry("empty", ""),
		Entry("path traversal", "../x"),
		Entry("slash", "a/b"),
		Entry("uppercase UUID", "0190B2A4-7F3C-7A52-8C1E-3D4F5A6B7C8D"),
		Entry("too few legacy parts", "a--b--c"),
		Entry("dot-dot in legacy id", "a--..--c--d"),
		Entry("percent-encoded traversal", "%2e%2e"),
		Entry("empty legacy segment", "a----c--d"),
		Entry("whitespace", "a--b--c--d e"),
		Entry("plain word", "readme"),
		Entry("overlong legacy id", "a--b--c--"+strings.Repeat("d", 600)),
	)
})
