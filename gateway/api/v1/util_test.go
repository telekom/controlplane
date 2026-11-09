// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseUpstream", func() {
	It("maps the URL components", func() {
		upstream, err := gatewayv1.ParseUpstream("https://provider.example.com:8443/api/v1")

		Expect(err).ToNot(HaveOccurred())
		Expect(upstream).To(Equal(gatewayv1.Upstream{
			Scheme:   "https",
			Hostname: "provider.example.com",
			Port:     8443,
			Path:     "/api/v1",
		}))
	})

	It("uses the default port of the scheme", func() {
		upstream, err := gatewayv1.ParseUpstream("http://provider.example.com/api")

		Expect(err).ToNot(HaveOccurred())
		Expect(upstream.Port).To(Equal(int32(80)))
	})

	It("returns an error for an invalid URL", func() {
		_, err := gatewayv1.ParseUpstream("http://provider.example.com/%zz")

		Expect(err).To(MatchError(ContainSubstring("failed to parse upstream URL")))
	})

	DescribeTable("keeps the percent-encoding of the path",
		func(rawUrl, expectedPath string) {
			upstream, err := gatewayv1.ParseUpstream(rawUrl)

			Expect(err).ToNot(HaveOccurred())
			Expect(upstream.Path).To(Equal(expectedPath))
		},
		Entry("encoded spaces",
			"https://provider.example.com/api/url%20encoded%20path", "/api/url%20encoded%20path"),
		Entry("encoded slash",
			"https://provider.example.com/files/a%2Fb", "/files/a%2Fb"),
		Entry("encoded percent sign",
			"https://provider.example.com/discounts/100%25", "/discounts/100%25"),
		Entry("double-encoded space",
			"https://provider.example.com/items/a%2520b", "/items/a%2520b"),
		Entry("path without encoding",
			"https://provider.example.com/api/v1", "/api/v1"),
	)
})
