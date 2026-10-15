// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DynamicUpstreamFeature", func() {
	var (
		ctx     context.Context
		f       *feature.DynamicUpstreamFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceDynamicUpstreamFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeDynamicUpstream", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeDynamicUpstream))
		})
	})

	Describe("Priority()", func() {
		It("returns 101", func() {
			Expect(f.Priority()).To(Equal(101))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route has 1 upstream with hostname localhost and DynamicUpstream configured on a non-proxy route", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "http", Hostname: "localhost", Port: 8080, Path: "/proxy"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has multiple upstreams", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "http", Hostname: "localhost", Port: 8080, Path: "/proxy"},
								{Scheme: "https", Hostname: "api.example.com", Port: 443, Path: "/v1"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when upstream hostname is not localhost", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api.example.com", Port: 443, Path: "/v1"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route is a proxy route", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypeProxy,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "http", Hostname: "localhost", Port: 8080, Path: "/proxy"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when DynamicUpstream is not configured in traffic", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "http", Hostname: "localhost", Port: 8080, Path: "/proxy"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: nil,
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no route in builder", func() {
			It("returns false", func() {
				builder.EXPECT().GetRoute().Return(nil, false)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})
	})

	Describe("Apply()", func() {
		Context("happy path", func() {
			It("overrides remote_api_url with dynamic query param reference and removes the query parameter", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "http", Hostname: "localhost", Port: 8080, Path: "/proxy"},
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}

				// Simulate what LastMileSecurity would have done: create RTP and pre-populate remote_api_url
				rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
				rtpPlugin.Config.Append.AddHeader("remote_api_url", "https://static-upstream.example.com:443/v1")

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				// The static remote_api_url should be replaced with the dynamic query parameter reference
				Expect(rtpPlugin.Config.Append.Headers).ToNot(BeNil())
				Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("$(query_params.target_url)"))

				// The query parameter should be added to the remove list
				Expect(rtpPlugin.Config.Remove.Querystring).ToNot(BeNil())
				Expect(rtpPlugin.Config.Remove.Querystring.Contains("target_url")).To(BeTrue())
			})
		})

		Context("error handling", func() {
			Context("when no route in builder", func() {
				It("returns ErrNoRoute", func() {
					builder.EXPECT().GetRoute().Return(nil, false)

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoRoute))
				})
			})
		})
	})
})
