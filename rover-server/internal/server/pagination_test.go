// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/url"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPagination(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pagination links")
}

var _ = Describe("Pagination links", func() {
	It("encodes cursors and retains page size and other query parameters", func() {
		query := url.Values{"limit": {"2"}, "cursor": {"previous"}, "shallowInfo": {"true"}}
		link := buildPageURL("https://api.example", "/rovers/info", query, "team/a&b c")
		u, err := url.Parse(link)
		Expect(err).NotTo(HaveOccurred())
		Expect(u.Query().Get("cursor")).To(Equal("team/a&b c"))
		Expect(u.Query().Get("limit")).To(Equal("2"))
		Expect(u.Query().Get("shallowInfo")).To(Equal("true"))
		Expect(buildPageURL("https://api.example", "/rovers", url.Values{}, "")).To(Equal("https://api.example/rovers?limit=20"))
	})
})
