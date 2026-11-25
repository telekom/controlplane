// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	clientpkg "github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewUpstream", func() {
	DescribeTable("keeps the percent-encoding of the path",
		func(rawUrl, expectedPath string) {
			upstream, err := clientpkg.NewUpstream(rawUrl)

			Expect(err).ToNot(HaveOccurred())
			Expect(upstream.GetPath()).To(Equal(expectedPath))
		},
		Entry("encoded spaces",
			"https://provider.example.com:443/api/url%20encoded%20path", "/api/url%20encoded%20path"),
		Entry("encoded slash",
			"https://provider.example.com:443/files/a%2Fb", "/files/a%2Fb"),
		Entry("encoded percent sign",
			"https://provider.example.com:443/discounts/100%25", "/discounts/100%25"),
		Entry("double-encoded space",
			"https://provider.example.com:443/items/a%2520b", "/items/a%2520b"),
		Entry("path without encoding",
			"http://localhost:8080/proxy", "/proxy"),
	)
})
