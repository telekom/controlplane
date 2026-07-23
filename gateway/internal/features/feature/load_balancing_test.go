// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	"github.com/stretchr/testify/mock"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadBalancingFeature", func() {
	var (
		ctx     context.Context
		f       *feature.LoadBalancingFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceLoadBalancingFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeLoadBalancing", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeLoadBalancing))
		})
	})

	Describe("Priority()", func() {
		It("returns 102", func() {
			Expect(f.Priority()).To(Equal(102))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route has more than one upstream", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api-a.example.com", Port: 443, Path: "/v1", Weight: 70},
								{Scheme: "https", Hostname: "api-b.example.com", Port: 443, Path: "/v1", Weight: 30},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has exactly one upstream", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api.example.com", Port: 443, Path: "/v1"},
							},
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
			It("sets upstream to localhost proxy, populates JumperConfig LoadBalancing with servers and weights", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: true, // LMS not used → remote_api_url should not be removed
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api-a.example.com", Port: 443, Path: "/v1", Weight: 70},
								{Scheme: "https", Hostname: "api-b.example.com", Port: 8443, Path: "/v2", Weight: 30},
							},
						},
					},
				}

				rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
				jumperConfig := plugin.NewJumperConfig()

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) {
					Expect(u.GetScheme()).To(Equal("http"))
					Expect(u.GetHostname()).To(Equal("localhost"))
					Expect(u.GetPort()).To(Equal(8080))
					Expect(u.GetPath()).To(Equal("/proxy"))
				}).Return()
				builder.EXPECT().JumperConfig().Return(jumperConfig)
				builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				// Verify LoadBalancing servers
				Expect(jumperConfig.LoadBalancing).ToNot(BeNil())
				Expect(jumperConfig.LoadBalancing.Servers).To(HaveLen(2))
				Expect(jumperConfig.LoadBalancing.Servers[0].Upstream).To(Equal("https://api-a.example.com:443/v1"))
				Expect(jumperConfig.LoadBalancing.Servers[0].Weight).To(Equal(int32(70)))
				Expect(jumperConfig.LoadBalancing.Servers[1].Upstream).To(Equal("https://api-b.example.com:8443/v2"))
				Expect(jumperConfig.LoadBalancing.Servers[1].Weight).To(Equal(int32(30)))
			})

			Context("when route is primary and Last Mile Security is active", func() {
				It("removes remote_api_url from the RTP append headers", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type:        gatewayv1.RouteTypePrimary,
							PassThrough: false, // LMS is active
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "api-a.example.com", Port: 443, Path: "/v1", Weight: 50},
									{Scheme: "https", Hostname: "api-b.example.com", Port: 443, Path: "/v1", Weight: 50},
								},
							},
						},
					}

					// Simulate LastMileSecurity having set remote_api_url
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					rtpPlugin.Config.Append.AddHeader("remote_api_url", "https://api-a.example.com:443/v1")

					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// remote_api_url should be removed because LMS is active on a real route
					Expect(rtpPlugin.Config.Append.Headers.Contains("remote_api_url")).To(BeFalse())
				})
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

		Context("edge cases", func() {
			Context("when route is a proxy route", func() {
				It("does NOT remove remote_api_url from RTP", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type:        gatewayv1.RouteTypeProxy,
							PassThrough: false, // LMS would be active, but route is proxy
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "api-a.example.com", Port: 443, Path: "/v1", Weight: 60},
									{Scheme: "https", Hostname: "api-b.example.com", Port: 443, Path: "/v1", Weight: 40},
								},
							},
						},
					}

					// Simulate LastMileSecurity having set remote_api_url
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					rtpPlugin.Config.Append.AddHeader("remote_api_url", "https://api-a.example.com:443/v1")

					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// remote_api_url should NOT be removed because it's a proxy route
					Expect(rtpPlugin.Config.Append.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Append.Headers.Contains("remote_api_url")).To(BeTrue())
					Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("https://api-a.example.com:443/v1"))
				})
			})
		})
	})
})
