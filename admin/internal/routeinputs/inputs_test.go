// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package routeinputs_test

import (
	"testing"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/admin/internal/routeinputs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRouteInputs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Route Inputs Suite")
}

var _ = Describe("Route inputs", func() {
	It("aggregates only the selected gateway's presets, including hidden URLs", func() {
		spec := &adminv1.ZoneSpec{Presets: []adminv1.Preset{
			{GatewayRef: "standard", Urls: []adminv1.UrlConfig{
				{Hostname: "z.example.com", BasePath: "/v2"},
				{Hostname: "a.example.com", BasePath: "/v1", Hidden: true},
			}},
			{GatewayRef: "other", Urls: []adminv1.UrlConfig{{Hostname: "other.example.com", BasePath: "/other"}}},
			{GatewayRef: "standard", Urls: []adminv1.UrlConfig{{Hostname: "z.example.com", BasePath: "/v1"}}},
		}}
		hostnames, basePaths := routeinputs.HostnamesAndBasePaths(spec, "standard")
		Expect(hostnames).To(Equal([]string{"a.example.com", "z.example.com"}))
		Expect(basePaths).To(Equal([]string{"/v1", "/v2"}))
	})

	DescribeTable("normalizes paths consistently for health and identity suffixes",
		func(suffix string) {
			basePaths := []string{"/v2/", "/v1", "/v1/", "/v1/./", "/nested/../v1", "/v2"}
			Expect(routeinputs.Paths(basePaths, suffix)).To(Equal([]string{"/v1" + suffix, "/v2" + suffix}))
		},
		Entry("health", routeinputs.ZoneHealthPath),
		Entry("issuer", "/auth/realms/default"),
		Entry("internal certs", "/auth/realms/rover/protocol/openid-connect/certs"),
		Entry("team discovery", "/auth/realms/team-api/.well-known/openid-configuration"),
		Entry("world issuer", "/spacegate/auth/realms/default"),
	)

	It("reports both missing hostnames and paths for an unreferenced gateway", func() {
		errs := routeinputs.Validate(&adminv1.ZoneSpec{Gateways: []adminv1.GatewayConfig{{Name: "unused"}}})
		Expect(errs).To(HaveLen(2))
		for _, err := range errs {
			Expect(err.Field).To(Equal("spec.gateways[0].name"))
		}
		Expect(errs.ToAggregate()).To(MatchError(ContainSubstring("hostnames")))
		Expect(errs.ToAggregate()).To(MatchError(ContainSubstring("paths")))
	})
})
