// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("Route", func() {
	Describe("GetHosts", func() {
		DescribeTable("should return unique hosts from downstreams",
			func(downstreams []gatewayv1.Downstream, expected []string) {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Downstreams: downstreams,
					},
				}
				Expect(route.GetHosts()).To(Equal(expected))
			},
			Entry("single host",
				[]gatewayv1.Downstream{
					{Host: "api.example.com", Path: "/v1"},
				},
				[]string{"api.example.com"},
			),
			Entry("multiple distinct hosts",
				[]gatewayv1.Downstream{
					{Host: "api1.example.com", Path: "/v1"},
					{Host: "api2.example.com", Path: "/v1"},
					{Host: "api3.example.com", Path: "/v1"},
				},
				[]string{"api1.example.com", "api2.example.com", "api3.example.com"},
			),
			Entry("duplicate hosts - should deduplicate",
				[]gatewayv1.Downstream{
					{Host: "api.example.com", Path: "/v1"},
					{Host: "api.example.com", Path: "/v2"},
					{Host: "api.example.com", Path: "/v3"},
				},
				[]string{"api.example.com"},
			),
			Entry("mixed duplicate and unique hosts - preserves order",
				[]gatewayv1.Downstream{
					{Host: "api1.example.com", Path: "/v1"},
					{Host: "api2.example.com", Path: "/v1"},
					{Host: "api1.example.com", Path: "/v2"}, // Duplicate
					{Host: "api3.example.com", Path: "/v1"},
					{Host: "api2.example.com", Path: "/v2"}, // Duplicate
				},
				[]string{"api1.example.com", "api2.example.com", "api3.example.com"},
			),
			Entry("empty downstreams",
				[]gatewayv1.Downstream{},
				[]string{},
			),
		)
	})

	Describe("GetPaths", func() {
		DescribeTable("should return unique paths from downstreams",
			func(downstreams []gatewayv1.Downstream, expected []string) {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Downstreams: downstreams,
					},
				}
				Expect(route.GetPaths()).To(Equal(expected))
			},
			Entry("single path",
				[]gatewayv1.Downstream{
					{Host: "api.example.com", Path: "/v1"},
				},
				[]string{"/v1"},
			),
			Entry("multiple distinct paths",
				[]gatewayv1.Downstream{
					{Host: "api.example.com", Path: "/v1"},
					{Host: "api.example.com", Path: "/v2"},
					{Host: "api.example.com", Path: "/v3"},
				},
				[]string{"/v1", "/v2", "/v3"},
			),
			Entry("duplicate paths - should deduplicate",
				[]gatewayv1.Downstream{
					{Host: "api1.example.com", Path: "/api/v1"},
					{Host: "api2.example.com", Path: "/api/v1"},
					{Host: "api3.example.com", Path: "/api/v1"},
				},
				[]string{"/api/v1"},
			),
			Entry("mixed duplicate and unique paths - preserves order",
				[]gatewayv1.Downstream{
					{Host: "api.example.com", Path: "/v1"},
					{Host: "api.example.com", Path: "/v2"},
					{Host: "api.example.com", Path: "/v1"}, // Duplicate
					{Host: "api.example.com", Path: "/v3"},
					{Host: "api.example.com", Path: "/v2"}, // Duplicate
				},
				[]string{"/v1", "/v2", "/v3"},
			),
			Entry("empty downstreams",
				[]gatewayv1.Downstream{},
				[]string{},
			),
		)
	})

	Describe("DTC scenarios", func() {
		Context("DTC realm with multiple zone URLs", func() {
			It("should return multiple hosts but single deduplicated path", func() {
				// Simulates a DTC realm route with multiple gateway URLs
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Downstreams: []gatewayv1.Downstream{
							{Host: "gateway-zone-a.example.com", Path: "/realm/dtc"},
							{Host: "gateway-zone-b.example.com", Path: "/realm/dtc"},
							{Host: "gateway-zone-c.example.com", Path: "/realm/dtc"},
						},
					},
				}

				hosts := route.GetHosts()
				paths := route.GetPaths()

				// Should have 3 unique hosts
				Expect(hosts).To(HaveLen(3))
				Expect(hosts).To(ContainElements("gateway-zone-a.example.com", "gateway-zone-b.example.com", "gateway-zone-c.example.com"))

				// Should have only 1 path (deduplicated)
				Expect(paths).To(Equal([]string{"/realm/dtc"}))
			})
		})

		Context("Cross-zone proxy route with different paths", func() {
			It("should return all unique hosts and paths", func() {
				// Simulates a route with different paths for different zones
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Downstreams: []gatewayv1.Downstream{
							{Host: "api-zone-a.example.com", Path: "/api/v1"},
							{Host: "api-zone-b.example.com", Path: "/api/v2"},
							{Host: "api-zone-c.example.com", Path: "/api/v1"}, // Same path as zone-a
						},
					},
				}

				hosts := route.GetHosts()
				paths := route.GetPaths()

				// Should have 3 unique hosts
				Expect(hosts).To(HaveLen(3))
				Expect(hosts).To(ContainElements("api-zone-a.example.com", "api-zone-b.example.com", "api-zone-c.example.com"))

				// Should have 2 unique paths (v1 and v2)
				Expect(paths).To(HaveLen(2))
				Expect(paths).To(ContainElements("/api/v1", "/api/v2"))
			})
		})
	})
})
