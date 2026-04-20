// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared_test

import (
	"github.com/telekom/controlplane/projector/internal/domain/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TeamNameFromNamespace", func() {
	It("strips the environment prefix", func() {
		Expect(shared.TeamNameFromNamespace("prod--eni--narvi-regr")).To(Equal("eni--narvi-regr"))
	})

	It("handles two-segment namespace", func() {
		Expect(shared.TeamNameFromNamespace("dev--my-team")).To(Equal("my-team"))
	})

	It("returns full namespace when no separator is found", func() {
		Expect(shared.TeamNameFromNamespace("simple-ns")).To(Equal("simple-ns"))
	})

	It("handles empty namespace", func() {
		Expect(shared.TeamNameFromNamespace("")).To(BeEmpty())
	})

	It("handles separator at the beginning", func() {
		Expect(shared.TeamNameFromNamespace("--rest")).To(Equal("rest"))
	})

	It("handles separator at the end", func() {
		Expect(shared.TeamNameFromNamespace("prefix--")).To(BeEmpty())
	})

	It("only strips the first segment", func() {
		Expect(shared.TeamNameFromNamespace("env--group--team")).To(Equal("group--team"))
	})
})
