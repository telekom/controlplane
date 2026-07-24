// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/kong/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("RateLimitFeature", func() {

	var (
		ctx     context.Context
		f       *feature.RateLimitFeature
		builder *featmock.MockKongFeatureBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceRateLimitFeature
		builder = featmock.NewMockKongFeatureBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeRateLimit", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeRateLimit))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		var gatewayWithRedis *gatewayv1.Gateway

		BeforeEach(func() {
			gatewayWithRedis = &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Redis: &gatewayv1.RedisConfig{
						Host: "redis.example.com",
						Port: 6379,
					},
				},
			}
		})

		Context("when route has RateLimit configured", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{Second: 10, Minute: 100, Hour: 1000},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayWithRedis)
				builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when consumer has RateLimit and references this route", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{},
					},
				}
				consumers := []*gatewayv1.ConsumeRoute{
					{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
							ConsumerName: "consumer-a",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 5},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayWithRedis)
				builder.EXPECT().GetAllowedConsumers().Return(consumers)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when consumer has RateLimit but references different route", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{},
					},
				}
				consumers := []*gatewayv1.ConsumeRoute{
					{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "other-route", Namespace: "other-ns"},
							ConsumerName: "consumer-a",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 5},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayWithRedis)
				builder.EXPECT().GetAllowedConsumers().Return(consumers)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route is passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{Second: 10},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayWithRedis)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no rate limit anywhere", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayWithRedis)
				builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when Redis is not configured in gateway", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-ns",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							RateLimit: &gatewayv1.RateLimit{
								Limits: gatewayv1.Limits{Second: 10},
							},
						},
					},
				}
				gatewayNoRedis := &gatewayv1.Gateway{
					Spec: gatewayv1.GatewaySpec{},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetGateway().Return(gatewayNoRedis)

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
		var gateway *gatewayv1.Gateway

		BeforeEach(func() {
			gateway = &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Spec: gatewayv1.GatewaySpec{
					Redis: &gatewayv1.RedisConfig{
						Host:      "redis.example.com",
						Port:      6379,
						Password:  "redis-secret-password",
						EnableTLS: true,
					},
				},
			}
		})

		Context("happy path", func() {
			Context("when primary route has rate limit", func() {
				It("sets policy=redis, redis config, service limits, and options", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{
								RateLimit: &gatewayv1.RateLimit{
									Limits: gatewayv1.Limits{Second: 10, Minute: 100, Hour: 1000},
									Options: gatewayv1.RateLimitOptions{
										HideClientHeaders: true,
										FaultTolerant:     false,
									},
								},
							},
						},
					}
					rateLimitPlugin := plugin.RateLimitPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RateLimitPluginRoute().Return(rateLimitPlugin)
					builder.EXPECT().GetGateway().Return(gateway)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
					Expect(rateLimitPlugin.Config.RedisConfig.Host).To(Equal("redis.example.com"))
					Expect(rateLimitPlugin.Config.RedisConfig.Port).To(Equal(6379))
					Expect(rateLimitPlugin.Config.RedisConfig.Password).To(Equal("redis-secret-password"))
					Expect(rateLimitPlugin.Config.RedisConfig.Ssl).To(BeTrue())
					Expect(rateLimitPlugin.Config.OmitConsumer).To(Equal("gateway"))
					Expect(rateLimitPlugin.Config.Limits.Service).ToNot(BeNil())
					Expect(rateLimitPlugin.Config.Limits.Service.Second).To(Equal(10))
					Expect(rateLimitPlugin.Config.Limits.Service.Minute).To(Equal(100))
					Expect(rateLimitPlugin.Config.Limits.Service.Hour).To(Equal(1000))
					Expect(rateLimitPlugin.Config.HideClientHeaders).To(BeTrue())
					Expect(rateLimitPlugin.Config.FaultTolerant).To(BeFalse())
				})
			})

			Context("when consumer has rate limit on primary route", func() {
				It("consumer plugin has consumer limits, service limits, and options", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{
								RateLimit: &gatewayv1.RateLimit{
									Limits: gatewayv1.Limits{Second: 10, Minute: 100, Hour: 1000},
									Options: gatewayv1.RateLimitOptions{
										HideClientHeaders: true,
										FaultTolerant:     true,
									},
								},
							},
						},
					}
					consumer := &gatewayv1.ConsumeRoute{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
							ConsumerName: "consumer-a",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 5, Minute: 50, Hour: 500},
								},
							},
						},
					}

					routePlugin := plugin.RateLimitPluginFromRoute(route)
					consumerPlugin := plugin.RateLimitPluginFromConsumeRoute(consumer)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RateLimitPluginRoute().Return(routePlugin)
					builder.EXPECT().GetGateway().Return(gateway)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
					builder.EXPECT().RateLimitPluginConsumeRoute(consumer).Return(consumerPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Consumer plugin should have consumer limits
					Expect(consumerPlugin.Config.Limits.Consumer).ToNot(BeNil())
					Expect(consumerPlugin.Config.Limits.Consumer.Second).To(Equal(5))
					Expect(consumerPlugin.Config.Limits.Consumer.Minute).To(Equal(50))
					Expect(consumerPlugin.Config.Limits.Consumer.Hour).To(Equal(500))

					// Consumer plugin should also have service limits (primary route)
					Expect(consumerPlugin.Config.Limits.Service).ToNot(BeNil())
					Expect(consumerPlugin.Config.Limits.Service.Second).To(Equal(10))
					Expect(consumerPlugin.Config.Limits.Service.Minute).To(Equal(100))
					Expect(consumerPlugin.Config.Limits.Service.Hour).To(Equal(1000))

					// Options from route should be applied
					Expect(consumerPlugin.Config.HideClientHeaders).To(BeTrue())
					Expect(consumerPlugin.Config.FaultTolerant).To(BeTrue())
				})
			})

			Context("when consumer has rate limit on proxy route", func() {
				It("consumer plugin has consumer limits only without service limits", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "proxy-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypeProxy,
							Traffic: gatewayv1.Traffic{
								// Proxy route has no rate limit
							},
						},
					}
					consumer := &gatewayv1.ConsumeRoute{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "proxy-route", Namespace: "test-ns"},
							ConsumerName: "consumer-a",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 3, Minute: 30, Hour: 300},
								},
							},
						},
					}

					consumerPlugin := plugin.RateLimitPluginFromConsumeRoute(consumer)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetGateway().Return(gateway)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
					builder.EXPECT().RateLimitPluginConsumeRoute(consumer).Return(consumerPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Consumer plugin should have consumer limits
					Expect(consumerPlugin.Config.Limits.Consumer).ToNot(BeNil())
					Expect(consumerPlugin.Config.Limits.Consumer.Second).To(Equal(3))
					Expect(consumerPlugin.Config.Limits.Consumer.Minute).To(Equal(30))
					Expect(consumerPlugin.Config.Limits.Consumer.Hour).To(Equal(300))

					// No service limits on proxy route without rate limit
					Expect(consumerPlugin.Config.Limits.Service).To(BeNil())
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

			Context("when secret manager fails for redis password", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, _ string) (string, error) {
						return "", fmt.Errorf("secret resolution failed")
					}

					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{
								RateLimit: &gatewayv1.RateLimit{
									Limits: gatewayv1.Limits{Second: 10},
								},
							},
						},
					}
					rateLimitPlugin := plugin.RateLimitPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RateLimitPluginRoute().Return(rateLimitPlugin)
					builder.EXPECT().GetGateway().Return(gateway)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get redis password for gateway"))
					Expect(err.Error()).To(ContainSubstring("secret resolution failed"))
				})
			})
		})

		Context("edge cases", func() {
			Context("when consumer references a different route", func() {
				It("is skipped and no consumer plugin is created", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{
								RateLimit: &gatewayv1.RateLimit{
									Limits: gatewayv1.Limits{Second: 10, Minute: 100, Hour: 1000},
								},
							},
						},
					}
					nonMatchingConsumer := &gatewayv1.ConsumeRoute{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "other-route", Namespace: "other-ns"},
							ConsumerName: "consumer-x",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 5},
								},
							},
						},
					}
					routePlugin := plugin.RateLimitPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RateLimitPluginRoute().Return(routePlugin)
					builder.EXPECT().GetGateway().Return(gateway)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{nonMatchingConsumer})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Route plugin should be configured, but no consumer plugin call expected
					Expect(routePlugin.Config.Limits.Service).ToNot(BeNil())
					Expect(routePlugin.Config.Limits.Service.Second).To(Equal(10))
				})
			})

			Context("when proxy route has route-level rate limit and consumer with rate limit", func() {
				It("does not create route-level plugin but applies options to consumer plugin", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "proxy-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeProxy,
							Traffic: gatewayv1.Traffic{
								RateLimit: &gatewayv1.RateLimit{
									Limits: gatewayv1.Limits{Second: 20, Minute: 200},
									Options: gatewayv1.RateLimitOptions{
										HideClientHeaders: true,
										FaultTolerant:     false,
									},
								},
							},
						},
					}
					consumer := &gatewayv1.ConsumeRoute{
						Spec: gatewayv1.ConsumeRouteSpec{
							Route:        types.ObjectRef{Name: "proxy-route", Namespace: "test-ns"},
							ConsumerName: "consumer-a",
							Traffic: &gatewayv1.ConsumeRouteTraffic{
								RateLimit: &gatewayv1.ConsumeRouteRateLimit{
									Limits: gatewayv1.Limits{Second: 3, Minute: 30},
								},
							},
						},
					}

					consumerPlugin := plugin.RateLimitPluginFromConsumeRoute(consumer)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetGateway().Return(gateway)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
					builder.EXPECT().RateLimitPluginConsumeRoute(consumer).Return(consumerPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Consumer plugin has consumer limits
					Expect(consumerPlugin.Config.Limits.Consumer).ToNot(BeNil())
					Expect(consumerPlugin.Config.Limits.Consumer.Second).To(Equal(3))
					Expect(consumerPlugin.Config.Limits.Consumer.Minute).To(Equal(30))

					// No service limits because route is proxy (else-if branch applies options only)
					Expect(consumerPlugin.Config.Limits.Service).To(BeNil())

					// Options from route are applied via the else-if branch
					Expect(consumerPlugin.Config.HideClientHeaders).To(BeTrue())
					Expect(consumerPlugin.Config.FaultTolerant).To(BeFalse())
				})
			})
		})
	})
})
