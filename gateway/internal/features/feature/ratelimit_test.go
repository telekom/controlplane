// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

var _ = Describe("RateLimitFeature", func() {

	It("should return the correct feature type", func() {
		Expect(InstanceRateLimitFeature.Name()).To(Equal(gatewayv1.FeatureTypeRateLimit))
	})

	It("should have the correct priority", func() {
		Expect(InstanceRateLimitFeature.Priority()).To(Equal(10))
	})

	Context("with mocked feature builder", func() {
		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder
		var feature RateLimitFeature

		BeforeEach(func() {
			feature = RateLimitFeature{priority: 10}

			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
		})

		Context("check IsUsed", func() {
			It("should not be used when no route is available", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should not be used when route is pass-through", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{
									Second: 100,
									Minute: 1000,
									Hour:   10000,
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should not be used when route has no rate limit", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should be used when route has rate limit and is not pass-through", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{
									Second: 100,
									Minute: 1000,
									Hour:   10000,
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})
		})

		Context("Apply", func() {
			It("should do nothing when no route is available", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false)
				Expect(feature.Apply(context.Background(), mockFeatureBuilder)).To(Succeed())
			})

			It("should configure rate limit plugin for non-proxy route", func() {
				// Setup route with rate limits
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{
									Second: 100,
									Minute: 1000,
									Hour:   10000,
								},
								Options: gatewayv1.RateLimitOptions{
									HideClientHeaders: ptrBool(true),
									FaultTolerant:     ptrBool(false),
								},
							},
						},
					},
				}

				// Setup gateway with Redis configuration
				gateway := &gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{
						Redis: gatewayv1.RedisConfig{
							Host: "redis-host",
							Port: 6379,
						},
					},
				}

				// Setup rate limit plugin
				rateLimitPlugin := &plugin.RateLimitPlugin{
					Config: plugin.RateLimitPluginConfig{},
				}

				// Setup expectations
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().RateLimitPluginRoute().Return(rateLimitPlugin)
				mockFeatureBuilder.EXPECT().GetGateway().Return(gateway)

				// Apply feature
				Expect(feature.Apply(context.Background(), mockFeatureBuilder)).To(Succeed())

				// Verify rate limit plugin configuration
				Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
				Expect(rateLimitPlugin.Config.RedisConfig.Host).To(Equal("redis-host"))
				Expect(rateLimitPlugin.Config.RedisConfig.Port).To(Equal(6379))
				Expect(rateLimitPlugin.Config.OmitConsumer).To(Equal("gateway"))
				Expect(rateLimitPlugin.Config.HideClientHeaders).To(BeTrue())
				Expect(rateLimitPlugin.Config.FaultTolerant).To(BeFalse())
				Expect(rateLimitPlugin.Config.Limits.Service).NotTo(BeNil())
				Expect(rateLimitPlugin.Config.Limits.Service.Second).To(Equal(100))
				Expect(rateLimitPlugin.Config.Limits.Service.Minute).To(Equal(1000))
				Expect(rateLimitPlugin.Config.Limits.Service.Hour).To(Equal(10000))
			})

			It("should configure rate limit plugin for proxy route with consumer rate limits", func() {
				// Setup proxy route with rate limits
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Upstreams: []gatewayv1.Upstream{
							{
								IssuerUrl: "http://issuer", // Issuer URL indicates proxy route
							},
						},
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{
									Second: 100,
									Minute: 1000,
									Hour:   10000,
								},
								Options: gatewayv1.RateLimitOptions{
									HideClientHeaders: ptrBool(true),
									FaultTolerant:     ptrBool(false),
								},
							},
						},
					},
				}

				// Setup consume route with rate limits
				consumeRoute := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						Traffic: &gatewayv1.ConsumeRouteTraffic{
							RateLimit: &gatewayv1.ConsumeRouteRateLimit{
								Limits: gatewayv1.Limits{
									Second: 50,
									Minute: 500,
									Hour:   5000,
								},
							},
						},
					},
				}

				// Setup gateway with Redis configuration
				gateway := &gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{
						Redis: gatewayv1.RedisConfig{
							Host: "redis-host",
							Port: 6379,
						},
					},
				}

				// Setup rate limit plugin
				rateLimitPlugin := &plugin.RateLimitPlugin{
					Config: plugin.RateLimitPluginConfig{},
				}

				// Setup expectations
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumeRoute})
				mockFeatureBuilder.EXPECT().RateLimitPluginConsumeRoute(consumeRoute).Return(rateLimitPlugin)
				mockFeatureBuilder.EXPECT().GetGateway().Return(gateway)

				// Apply feature
				Expect(feature.Apply(context.Background(), mockFeatureBuilder)).To(Succeed())

				// Verify rate limit plugin configuration
				Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
				Expect(rateLimitPlugin.Config.RedisConfig.Host).To(Equal("redis-host"))
				Expect(rateLimitPlugin.Config.RedisConfig.Port).To(Equal(6379))
				Expect(rateLimitPlugin.Config.OmitConsumer).To(Equal("gateway"))
				Expect(rateLimitPlugin.Config.HideClientHeaders).To(BeTrue())
				Expect(rateLimitPlugin.Config.FaultTolerant).To(BeFalse())
				Expect(rateLimitPlugin.Config.Limits.Service).NotTo(BeNil())
				Expect(rateLimitPlugin.Config.Limits.Service.Second).To(Equal(100))
				Expect(rateLimitPlugin.Config.Limits.Service.Minute).To(Equal(1000))
				Expect(rateLimitPlugin.Config.Limits.Service.Hour).To(Equal(10000))
				Expect(rateLimitPlugin.Config.Limits.Consumer).NotTo(BeNil())
				Expect(rateLimitPlugin.Config.Limits.Consumer.Second).To(Equal(50))
				Expect(rateLimitPlugin.Config.Limits.Consumer.Minute).To(Equal(500))
				Expect(rateLimitPlugin.Config.Limits.Consumer.Hour).To(Equal(5000))
			})

			It("should configure rate limit plugin for proxy route with only route rate limits", func() {
				// Setup proxy route with rate limits
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Upstreams: []gatewayv1.Upstream{
							{
								IssuerUrl: "http://issuer", // Issuer URL indicates proxy route
							},
						},
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{
									Second: 100,
									Minute: 1000,
									Hour:   10000,
								},
							},
						},
					},
				}

				// Setup consume route without rate limits
				consumeRoute := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{},
				}

				// Setup gateway with Redis configuration
				gateway := &gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{
						Redis: gatewayv1.RedisConfig{
							Host: "redis-host",
							Port: 6379,
						},
					},
				}

				// Setup rate limit plugin
				rateLimitPlugin := &plugin.RateLimitPlugin{
					Config: plugin.RateLimitPluginConfig{},
				}

				// Setup expectations
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumeRoute})
				mockFeatureBuilder.EXPECT().RateLimitPluginConsumeRoute(consumeRoute).Return(rateLimitPlugin)
				mockFeatureBuilder.EXPECT().GetGateway().Return(gateway)

				// Apply feature
				Expect(feature.Apply(context.Background(), mockFeatureBuilder)).To(Succeed())

				// Verify rate limit plugin configuration
				Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
				Expect(rateLimitPlugin.Config.Limits.Service).NotTo(BeNil())
				Expect(rateLimitPlugin.Config.Limits.Service.Second).To(Equal(100))
				Expect(rateLimitPlugin.Config.Limits.Consumer).To(BeNil())
			})

			It("should configure rate limit plugin for proxy route with only consumer rate limits", func() {
				// Setup proxy route without rate limits
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Upstreams: []gatewayv1.Upstream{
							{
								IssuerUrl: "http://issuer", // Issuer URL indicates proxy route
							},
						},
					},
				}

				// Setup consume route with rate limits
				consumeRoute := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						Traffic: &gatewayv1.ConsumeRouteTraffic{
							RateLimit: &gatewayv1.ConsumeRouteRateLimit{
								Limits: gatewayv1.Limits{
									Second: 50,
									Minute: 500,
									Hour:   5000,
								},
							},
						},
					},
				}

				// Setup gateway with Redis configuration
				gateway := &gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{
						Redis: gatewayv1.RedisConfig{
							Host: "redis-host",
							Port: 6379,
						},
					},
				}

				// Setup rate limit plugin
				rateLimitPlugin := &plugin.RateLimitPlugin{
					Config: plugin.RateLimitPluginConfig{},
				}

				// Setup expectations
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumeRoute})
				mockFeatureBuilder.EXPECT().RateLimitPluginConsumeRoute(consumeRoute).Return(rateLimitPlugin)
				mockFeatureBuilder.EXPECT().GetGateway().Return(gateway)

				// Apply feature
				Expect(feature.Apply(context.Background(), mockFeatureBuilder)).To(Succeed())

				// Verify rate limit plugin configuration
				Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
				Expect(rateLimitPlugin.Config.Limits.Consumer).NotTo(BeNil())
				Expect(rateLimitPlugin.Config.Limits.Consumer.Second).To(Equal(50))
				Expect(rateLimitPlugin.Config.Limits.Service).To(BeNil())
			})
		})
	})
})

// Helper function to return a pointer to a bool
func ptrBool(b bool) *bool {
	return &b
}
