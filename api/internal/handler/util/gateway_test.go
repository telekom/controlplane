// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AsUpstream", func() {
	It("maps the URL components and the weight", func() {
		upstream, err := AsUpstream("https://provider.example.com:8443/api/v1", 40)

		Expect(err).ToNot(HaveOccurred())
		Expect(upstream).To(Equal(gatewayapi.Upstream{
			Scheme:   "https",
			Hostname: "provider.example.com",
			Port:     8443,
			Path:     "/api/v1",
			Weight:   40,
		}))
	})

	DescribeTable("keeps the percent-encoding of the path",
		func(rawUrl, expectedPath string) {
			upstream, err := AsUpstream(rawUrl, 0)

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
