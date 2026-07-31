// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
)

var _ = Describe("SubstitutePlaceholders", func() {

	Context("when content has no placeholders", func() {
		It("should return the content unchanged", func() {
			input := "plain text without any variables"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(input))
		})
	})

	Context("when content has a single placeholder", func() {
		It("should replace it with the env var value", func() {
			os.Setenv("TEST_PLACEHOLDER_VAR", "resolved-value")
			defer os.Unsetenv("TEST_PLACEHOLDER_VAR")

			input := "value: ${TEST_PLACEHOLDER_VAR}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value: resolved-value"))
		})
	})

	Context("when content has multiple placeholders", func() {
		It("should replace all of them", func() {
			os.Setenv("TEST_PH_A", "alpha")
			os.Setenv("TEST_PH_B", "beta")
			defer os.Unsetenv("TEST_PH_A")
			defer os.Unsetenv("TEST_PH_B")

			input := "first: ${TEST_PH_A}, second: ${TEST_PH_B}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("first: alpha, second: beta"))
		})
	})

	Context("when an env var is set to empty string", func() {
		It("should replace the placeholder with empty string", func() {
			os.Setenv("TEST_PH_EMPTY", "")
			defer os.Unsetenv("TEST_PH_EMPTY")

			input := "value: ${TEST_PH_EMPTY}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value: "))
		})
	})

	Context("when a placeholder references an unset variable", func() {
		It("should return an error listing the variable", func() {
			// Ensure the variable is definitely not set
			os.Unsetenv("TEST_PH_NONEXISTENT_XYZ_123")

			input := "value: ${TEST_PH_NONEXISTENT_XYZ_123}"
			_, err := parser.SubstitutePlaceholders(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unresolved environment variable(s)"))
			Expect(err.Error()).To(ContainSubstring("TEST_PH_NONEXISTENT_XYZ_123"))
		})
	})

	Context("when multiple placeholders reference unset variables", func() {
		It("should return an error listing all unresolved variables", func() {
			os.Unsetenv("TEST_PH_MISSING_A")
			os.Unsetenv("TEST_PH_MISSING_B")

			input := "a: ${TEST_PH_MISSING_A}, b: ${TEST_PH_MISSING_B}"
			_, err := parser.SubstitutePlaceholders(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TEST_PH_MISSING_A"))
			Expect(err.Error()).To(ContainSubstring("TEST_PH_MISSING_B"))
		})
	})

	Context("when content has dollar sign without braces", func() {
		It("should leave it unchanged", func() {
			input := "price: $100 and $VAR"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("price: $100 and $VAR"))
		})
	})

	Context("when content has invalid variable name in braces", func() {
		It("should leave it unchanged", func() {
			input := "value: ${123INVALID}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value: ${123INVALID}"))
		})
	})

	Context("when content has a placeholder with underscores", func() {
		It("should resolve it correctly", func() {
			os.Setenv("MY_LONG_VAR_NAME_123", "works")
			defer os.Unsetenv("MY_LONG_VAR_NAME_123")

			input := "value: ${MY_LONG_VAR_NAME_123}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value: works"))
		})
	})

	Context("when the same placeholder appears multiple times", func() {
		It("should replace all occurrences", func() {
			os.Setenv("TEST_PH_REPEATED", "val")
			defer os.Unsetenv("TEST_PH_REPEATED")

			input := "${TEST_PH_REPEATED} and ${TEST_PH_REPEATED}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("val and val"))
		})
	})

	Context("when used in a YAML-like structure", func() {
		It("should correctly substitute placeholders in various positions", func() {
			os.Setenv("TEST_PH_NAME", "my-rover")
			os.Setenv("TEST_PH_REPO", "https://github.com/example/repo")
			defer os.Unsetenv("TEST_PH_NAME")
			defer os.Unsetenv("TEST_PH_REPO")

			input := `apiVersion: tcp.ei.telekom.de/v1
kind: Rover
metadata:
  name: ${TEST_PH_NAME}
spec:
  repository: ${TEST_PH_REPO}`

			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("name: my-rover"))
			Expect(result).To(ContainSubstring("repository: https://github.com/example/repo"))
		})
	})

	Context("when a placeholder appears around YAML comments", func() {
		DescribeTable("should ignore placeholders only in valid YAML comments",
			func(input, expected string) {
				os.Unsetenv("TEST_PH_COMMENTED")

				result, err := parser.SubstitutePlaceholders(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(expected))
			},
			Entry("full-line comment",
				"# ${TEST_PH_COMMENTED}\nvalue: ok",
				"# ${TEST_PH_COMMENTED}\nvalue: ok"),
			Entry("inline comment",
				"value: ok # ${TEST_PH_COMMENTED}",
				"value: ok # ${TEST_PH_COMMENTED}"),
		)

		It("should still substitute the code portion of the line", func() {
			os.Setenv("TEST_PH_CODE", "resolved")
			defer os.Unsetenv("TEST_PH_CODE")
			os.Unsetenv("TEST_PH_COMMENTED")

			input := "value: ${TEST_PH_CODE} # ${TEST_PH_COMMENTED}"
			result, err := parser.SubstitutePlaceholders(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("value: resolved # ${TEST_PH_COMMENTED}"))
		})

		It("should replace a placeholder after a hash inside a quoted value", func() {
			os.Setenv("TEST_PH_QUOTED", "resolved")
			defer os.Unsetenv("TEST_PH_QUOTED")

			result, err := parser.SubstitutePlaceholders(`value: "text # ${TEST_PH_QUOTED}"`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(`value: "text # resolved"`))
		})

		DescribeTable("should replace placeholders after hashes in block scalars",
			func(indicator string) {
				os.Setenv("TEST_PH_BLOCK", "resolved")
				defer os.Unsetenv("TEST_PH_BLOCK")

				input := "value: " + indicator + "\n  text # ${TEST_PH_BLOCK}\n"
				result, err := parser.SubstitutePlaceholders(input)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("value: " + indicator + "\n  text # resolved\n"))
			},
			Entry("literal", "|"),
			Entry("folded", ">"),
		)
	})
})
