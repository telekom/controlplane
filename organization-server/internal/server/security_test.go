// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EnvironmentDecoder", func() {
	DescribeTable("decodes the environment",
		func(claims map[string]any, fallback, expected string) {
			actual, problem := EnvironmentDecoder(fallback)(claims, "env")
			Expect(problem).ToNot(HaveOccurred())
			Expect(actual).To(Equal(expected))
		},
		Entry("explicit env", map[string]any{"env": "prod"}, "fallback", "prod"),
		Entry("terminal team realm", map[string]any{"iss": "https://id.example/realms/team-dev"}, "fallback", "dev"),
		Entry("non-team realm uses fallback", map[string]any{"iss": "https://id.example/realms/master"}, "fallback", "fallback"),
		Entry("malformed issuer uses fallback", map[string]any{"iss": "://invalid"}, "fallback", "fallback"),
		Entry("empty team realm uses fallback", map[string]any{"iss": "https://id.example/realms/team-"}, "fallback", "fallback"),
		Entry("fallback", map[string]any{}, "fallback", "fallback"),
	)

	DescribeTable("rejects an empty environment",
		func(claims map[string]any, fallback string) {
			actual, problem := EnvironmentDecoder(fallback)(claims, "env")
			Expect(actual).To(BeEmpty())
			Expect(problem).To(HaveOccurred())
			Expect(problem.Code()).To(Equal(http.StatusUnauthorized))
		},
		Entry("non-terminal team realm", map[string]any{"iss": "https://id.example/realms/team-dev/extra"}, ""),
		Entry("empty fallback", map[string]any{}, ""),
	)
})
