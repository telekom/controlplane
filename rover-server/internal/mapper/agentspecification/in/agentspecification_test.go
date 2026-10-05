// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in_test

import (
	"context"

	"github.com/telekom/controlplane/rover-server/internal/mapper/agentspecification/in"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseSpecification", func() {
	DescribeTable("should set spec.name",
		func(specYAML, expected string) {
			spec, err := in.ParseSpecification(context.Background(), specYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec.Name).To(Equal("eni-assistant-v1"))
			Expect(spec.Spec.Name).To(Equal(expected))
		},
		Entry("to the title, unchanged",
			"basePath: /eni/assistant/v1\ninfo:\n  title: Weather Assistant\n", "Weather Assistant"),
		Entry("to the title without surrounding whitespace",
			"basePath: /eni/assistant/v1\ninfo:\n  title: \"  Weather Assistant \"\n", "Weather Assistant"),
		Entry("to the resource name when there is no title",
			"basePath: /eni/assistant/v1\ninfo:\n  version: 1.0.0\n", "eni-assistant-v1"),
		Entry("to the resource name when the title is blank",
			"basePath: /eni/assistant/v1\ninfo:\n  title: \"  \"\n", "eni-assistant-v1"),
		Entry("to the resource name when there is no info",
			"basePath: /eni/assistant/v1\n", "eni-assistant-v1"),
	)
})
