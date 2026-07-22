// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("routeDomains", func() {
	It("matches any Host when no hostnames are configured (RT-02)", func() {
		Expect(routeDomains(nil)).To(Equal([]string{"*"}))
	})

	It("uses the configured hostnames verbatim (RT-02)", func() {
		Expect(routeDomains([]string{"a.example.com", "b.example.com"})).
			To(Equal([]string{"a.example.com", "b.example.com"}))
	})
})

var _ = Describe("routeEntries", func() {
	It("emits a single match-all route when no paths are configured (RT-01)", func() {
		routes := routeEntries("cluster-x", nil, "")
		Expect(routes).To(HaveLen(1))
		Expect(routes[0].GetMatch().GetPrefix()).To(Equal("/"))
		Expect(routes[0].GetRoute().GetCluster()).To(Equal("cluster-x"))
		Expect(routes[0].GetRoute().GetRegexRewrite()).To(BeNil())
	})

	It("emits one route per path prefix, all to the same cluster (RT-01)", func() {
		routes := routeEntries("cluster-x", []string{"/api", "/v2"}, "")
		Expect(routes).To(HaveLen(2))
		prefixes := []string{}
		for _, r := range routes {
			prefixes = append(prefixes, r.GetMatch().GetPrefix())
			Expect(r.GetRoute().GetCluster()).To(Equal("cluster-x"))
		}
		Expect(prefixes).To(ConsistOf("/api", "/v2"))
	})

	It("prepends a non-trivial upstream base path via regex_rewrite (RV-04)", func() {
		routes := routeEntries("cluster-x", []string{"/api"}, "/backend")
		rw := routes[0].GetRoute().GetRegexRewrite()
		Expect(rw).NotTo(BeNil())
		Expect(rw.GetPattern().GetRegex()).To(Equal("^/"))
		Expect(rw.GetPattern().GetGoogleRe2()).NotTo(BeNil())
		Expect(rw.GetSubstitution()).To(Equal("/backend/"))
	})

	It("emits no rewrite for an empty or root base path (RV-04 identity guard)", func() {
		Expect(routeEntries("c", []string{"/api"}, "")[0].GetRoute().GetRegexRewrite()).To(BeNil())
		Expect(routeEntries("c", []string{"/api"}, "/")[0].GetRoute().GetRegexRewrite()).To(BeNil())
	})
})
