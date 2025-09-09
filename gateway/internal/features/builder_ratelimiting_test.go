// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewRateLimitRoute(isProxyRoute bool) *gatewayv1.Route {
	var issuerUrl string
	if isProxyRoute {
		issuerUrl = "https://upstream.issuer.url"
	} else {
		issuerUrl = ""
	}

	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "default",
		},
		Spec: gatewayv1.RouteSpec{
			Realm: types.ObjectRef{
				Name:      "realm",
				Namespace: "default",
			},
			PassThrough: false,
			Upstreams: []gatewayv1.Upstream{
				{
					Weight:    1,
					Scheme:    "http",
					Host:      "upstream.url",
					Port:      8080,
					Path:      "/api/v1",
					IssuerUrl: issuerUrl,
				},
			},
			Downstreams: []gatewayv1.Downstream{
				{
					Host:      "downstream.url",
					Port:      8080,
					Path:      "/test/v1",
					IssuerUrl: "issuer.url",
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
						HideClientHeaders: true,
						FaultTolerant:     false,
					},
				},
			},
		},
	}
}

func NewRateLimitConsumeRoute() *gatewayv1.ConsumeRoute {
	return &gatewayv1.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-consumer",
			Namespace: "default",
		},
		Spec: gatewayv1.ConsumeRouteSpec{
			ConsumerName: "test-consumer-name",
			Route: types.ObjectRef{
				Name:      "test-route",
				Namespace: "default",
			},
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
}

var _ = Describe("FeatureBuilder RateLimiting", Ordered, func() {
	var (
		ctx       context.Context
		gateway   *gatewayv1.Gateway
		testRealm *gatewayv1.Realm
	)

	ctx = context.Background()
	ctx = contextutil.WithEnv(ctx, "test")

	BeforeEach(func() {
		mockKc = mock.NewMockKongClient(mockCtrl)

		// Setup gateway with Redis configuration
		gateway = &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gateway",
				Namespace: "default",
			},
			Spec: gatewayv1.GatewaySpec{
				Redis: gatewayv1.RedisConfig{
					Host: "redis-host",
					Port: 6379,
				},
			},
		}

		// Setup realm
		testRealm = &gatewayv1.Realm{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "realm",
				Namespace: "default",
			},
			Spec: gatewayv1.RealmSpec{
				Url:       "https://realm.url",
				IssuerUrl: "https://issuer.url",
			},
		}
	})

	Context("Applying and Creating", Ordered, func() {
		It("should apply the RateLimit feature for a standard route", func() {
			rateLimitRoute := NewRateLimitRoute(false)

			By("building the features")
			builder := buildRateLimitFeature(ctx, rateLimitRoute, false, nil, gateway, testRealm)

			By("checking the rate limit plugin for route")
			rateLimitPlugin, ok := builder.Plugins["rate-limiting"].(*plugin.RateLimitPlugin)
			Expect(ok).To(BeTrue())
			Expect(rateLimitPlugin).NotTo(BeNil())
			Expect(rateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
			Expect(rateLimitPlugin.Config.RedisConfig.Host).To(Equal(gateway.Spec.Redis.Host))
			Expect(rateLimitPlugin.Config.RedisConfig.Port).To(Equal(gateway.Spec.Redis.Port))
			Expect(rateLimitPlugin.Config.Limits.Service).NotTo(BeNil())

			By("checking the rate limit plugin for route")
			Expect(rateLimitPlugin.Config.Limits.Service.Second).To(Equal(100))
			Expect(rateLimitPlugin.Config.Limits.Service.Minute).To(Equal(1000))
			Expect(rateLimitPlugin.Config.Limits.Service.Hour).To(Equal(10000))
			Expect(rateLimitPlugin.Config.HideClientHeaders).To(BeTrue())
			Expect(rateLimitPlugin.Config.FaultTolerant).To(BeFalse())
		})

		It("should apply the RateLimit feature for a proxy route with consumer rate limits", func() {
			rateLimitRoute := NewRateLimitRoute(true)
			consumeRoute := NewRateLimitConsumeRoute()

			By("building the features")
			builder := buildRateLimitFeature(ctx, rateLimitRoute, true, []*gatewayv1.ConsumeRoute{consumeRoute}, gateway, testRealm)

			By("checking the rate limit plugin for consumer")
			verifyRateLimitPluginConsumer(builder, consumeRoute, gateway, false)
		})

		It("should apply the RateLimit feature for a proxy route with only consumer rate limits", func() {
			rateLimitRoute := NewRateLimitRoute(true)
			rateLimitRoute.Spec.Traffic.RateLimit = nil
			consumeRoute := NewRateLimitConsumeRoute()

			By("building the features")
			builder := buildRateLimitFeature(ctx, rateLimitRoute, true, []*gatewayv1.ConsumeRoute{consumeRoute}, gateway, testRealm)

			By("checking the rate limit plugin for consumer")
			verifyRateLimitPluginConsumer(builder, consumeRoute, gateway, false)
		})

	})
})

func buildRateLimitFeature(ctx context.Context, route *gatewayv1.Route, isProxyRoute bool, consumeRoutes []*gatewayv1.ConsumeRoute, gateway *gatewayv1.Gateway, realm *gatewayv1.Realm) *features.Builder {

	builder := features.NewFeatureBuilder(mockKc, route, nil, realm, gateway)

	if consumeRoutes != nil {
		builder.AddAllowedConsumers(consumeRoutes...)
	}

	builder.EnableFeature(feature.InstanceRateLimitFeature)
	builder.SetUpstream(route.Spec.Upstreams[0])
	configureRouteLimitingMocks(ctx, route)

	err := builder.Build(ctx)
	Expect(err).NotTo(HaveOccurred())

	b, ok := builder.(*features.Builder)
	Expect(ok).To(BeTrue())
	return b
}

func verifyRateLimitPluginConsumer(builder *features.Builder, consumeRoute *gatewayv1.ConsumeRoute, gateway *gatewayv1.Gateway, serviceRateLimit bool) {
	// Find the rate limit plugin for the consumer
	var consumerRateLimitPlugin *plugin.RateLimitPlugin

	consumerName := consumeRoute.Spec.ConsumerName

	_, ok := builder.Plugins["rate-limiting-consumer--"+consumerName]
	Expect(ok).To(BeTrue())

	for _, p := range builder.Plugins {
		if rlp, ok := p.(*plugin.RateLimitPlugin); ok {
			if rlp.GetConsumer() != nil {
				consumerRateLimitPlugin = rlp
				break
			}
		}
	}

	Expect(consumerRateLimitPlugin).NotTo(BeNil())
	Expect(consumerRateLimitPlugin.Config.Policy).To(Equal(plugin.PolicyRedis))
	Expect(consumerRateLimitPlugin.Config.RedisConfig.Host).To(Equal(gateway.Spec.Redis.Host))
	Expect(consumerRateLimitPlugin.Config.RedisConfig.Port).To(Equal(gateway.Spec.Redis.Port))

	if serviceRateLimit {
		By("checking the rate limit plugin for provider")
		Expect(consumerRateLimitPlugin.Config.Limits.Service).NotTo(BeNil())
		Expect(consumerRateLimitPlugin.Config.Limits.Service.Second).To(Equal(100))
		Expect(consumerRateLimitPlugin.Config.Limits.Service.Minute).To(Equal(1000))
		Expect(consumerRateLimitPlugin.Config.Limits.Service.Hour).To(Equal(10000))
	}

	By("checking the rate limit plugin for consumer")
	Expect(consumerRateLimitPlugin.Config.Limits.Consumer).NotTo(BeNil())
	Expect(consumerRateLimitPlugin.Config.Limits.Consumer.Second).To(Equal(50))
	Expect(consumerRateLimitPlugin.Config.Limits.Consumer.Minute).To(Equal(500))
	Expect(consumerRateLimitPlugin.Config.Limits.Consumer.Hour).To(Equal(5000))
}

func configureRouteLimitingMocks(ctx context.Context, route *gatewayv1.Route) {
	mockKc.EXPECT().CreateOrReplaceRoute(ctx, route, gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
	mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}
