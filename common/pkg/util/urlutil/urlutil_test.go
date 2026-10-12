// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package urlutil

import (
	"net/url"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestURLUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "URLUtil Suite")
}

func mustParse(raw string) *url.URL {
	GinkgoHelper()
	u, err := url.Parse(raw)
	Expect(err).NotTo(HaveOccurred())
	return u
}

var _ = Describe("URL primitives", func() {
	DescribeTable("IsHTTPS",
		func(raw string, want bool) {
			Expect(IsHTTPS(mustParse(raw))).To(Equal(want))
		},
		Entry("https", "https://h/", true),
		Entry("upper case", "HTTPS://h/", true),
		Entry("http", "http://h/", false),
		Entry("relative", "/path", false),
	)

	DescribeTable("CanonicalHost",
		func(raw, want string) {
			Expect(CanonicalHost(mustParse(raw))).To(Equal(want))
		},
		Entry("lowercases", "https://Host.Example.COM/", "host.example.com"),
		Entry("drops default https port", "https://h:443/", "h"),
		Entry("drops default https port with upper-case scheme", "HTTPS://h:443/", "h"),
		Entry("drops default http port", "http://h:80/", "h"),
		Entry("keeps 443 for http", "http://h:443/", "h:443"),
		Entry("keeps 80 for https", "https://h:80/", "h:80"),
		Entry("keeps non-default port", "https://h:8443/", "h:8443"),
		Entry("brackets IPv6", "https://[::1]/", "[::1]"),
		Entry("brackets IPv6 with port", "https://[::1]:8443/", "[::1]:8443"),
		Entry("brackets IPv6 without default port", "https://[FE80::1]:443/", "[fe80::1]"),
		Entry("empty hostname with port", "https://:8443/", ":8443"),
		Entry("empty host", "https:///", ""),
	)
})
