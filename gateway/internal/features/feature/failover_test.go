// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Failover", func() {
	It("should have the correct priority", func() {
		Expect(feature.InstanceFailoverFeature.Priority()).To(Equal(feature.InstanceLastMileSecurityFeature.Priority() - 1))
	})

	Context("Correctly configure failover", func() {

		var ctx context.Context
		var mockCtrl *gomock.Controller

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			ctx = context.Background()
			ctx = contextutil.WithEnv(ctx, "test")
		})

		It("should apply failover feature when it is used", func() {
			mockKc := mock.NewMockKongClient(mockCtrl)

			route := NewRoute()
			route.Spec.Downstreams = []gatewayv1.Downstream{
				{
					Host:      "gateway1.example.com",
					Path:      "/foo",
					IssuerUrl: "https://issuer1.example.com",
				},
			}
			route.Spec.Upstreams = []gatewayv1.Upstream{
				{
					Scheme:    "http",
					Host:      "gateway2.example.com",
					Port:      80,
					IssuerUrl: "https://issuer2.example.com",
				},
			}
			route.Spec.Traffic = gatewayv1.Traffic{
				Failover: &gatewayv1.Failover{
					TargetZoneName: "zone1",
					Upstreams: []gatewayv1.Upstream{
						{
							Scheme: "http",
							Host:   "upstream1",
							Port:   80,
							Path:   "/",
						},
					},
				},
			}
			realm := NewRealm()
			gateway := NewGateway()

			// Expects
			mockCreateOrReplaceRoute := func(ctx context.Context, route client.CustomRoute, upstream client.Upstream, gateway *gatewayv1.Gateway) error {
				Expect(route.GetName()).To(Equal("test-route"))
				Expect(route.GetHost()).To(Equal("gateway1.example.com"))
				Expect(route.GetPath()).To(Equal("/foo"))
				Expect(upstream.GetHost()).To(Equal("localhost"))
				Expect(upstream.GetScheme()).To(Equal("http"))
				Expect(upstream.GetPort()).To(Equal(8080))
				Expect(upstream.GetPath()).To(Equal("/proxy"))
				return nil
			}
			mockKc.EXPECT().CreateOrReplaceRoute(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(mockCreateOrReplaceRoute).Times(1)

			mockCreateOrReplacePlugin := func(ctx context.Context, customPlugin client.CustomPlugin) (kongPlugin *kong.Plugin, err error) {
				switch p := customPlugin.(type) {
				case *plugin.RequestTransformerPlugin:
					Expect(p.GetName()).To(Equal("request-transformer"))
					b64str := p.Config.Append.Headers.Get(plugin.RoutingConfigKey)
					routingCfg, err := plugin.FromBase64[plugin.RoutingConfigs](b64str)
					if err != nil {
						Fail("Failed to decode routing config: " + err.Error())
					}
					Expect(routingCfg).ToNot(BeNil())
					Expect(routingCfg.Len()).To(Equal(2))

					normalUpstream := routingCfg.Get(0)
					Expect(normalUpstream.TargetZoneName).To(Equal("zone1"))
					Expect(normalUpstream.Issuer).To(Equal("https://issuer2.example.com"))
					Expect(normalUpstream.RemoteApiUrl).To(Equal("http://gateway2.example.com:80"))

					failoverUpstream := routingCfg.Get(1)
					Expect(failoverUpstream.TargetZoneName).To(Equal(""))
					Expect(failoverUpstream.Issuer).To(Equal(""))
					Expect(failoverUpstream.RemoteApiUrl).To(Equal("http://upstream1:80/"))

				default:
					Fail("Unexpected plugin type: " + p.GetName())
				}

				return nil, nil
			}
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).DoAndReturn(mockCreateOrReplacePlugin).Times(1)

			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

			builder := features.NewFeatureBuilder(mockKc, route, nil, realm, gateway)
			builder.EnableFeature(feature.InstanceFailoverFeature)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Correctly configure failover with loadbalancing", func() {

		var ctx context.Context
		var mockCtrl *gomock.Controller

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			ctx = context.Background()
			ctx = contextutil.WithEnv(ctx, "test")
		})

		It("should have the correct priority", func() {
			Expect(feature.InstanceFailoverFeature.Priority()).To(Equal(feature.InstanceLastMileSecurityFeature.Priority() - 1))
		})

		It("should apply failover feature when it is used", func() {
			mockKc := mock.NewMockKongClient(mockCtrl)

			route := NewRoute()
			route.Spec.Downstreams = []gatewayv1.Downstream{
				{
					Host:      "gateway1.example.com",
					Path:      "/foo",
					IssuerUrl: "https://issuer1.example.com",
				},
			}
			route.Spec.Upstreams = []gatewayv1.Upstream{
				{
					Scheme:    "http",
					Host:      "gateway2.example.com",
					Port:      80,
					IssuerUrl: "https://issuer2.example.com",
				},
			}
			route.Spec.Traffic = gatewayv1.Traffic{
				Failover: &gatewayv1.Failover{
					TargetZoneName: "zone1",
					Upstreams: []gatewayv1.Upstream{
						{
							Scheme: "http",
							Host:   "upstream1",
							Port:   80,
							Path:   "/",
							Weight: 50,
						},
						{
							Scheme: "http",
							Host:   "upstream2",
							Port:   80,
							Path:   "/",
							Weight: 50,
						},
					},
				},
			}
			realm := NewRealm()
			gateway := NewGateway()

			// Expects
			mockCreateOrReplaceRoute := func(ctx context.Context, route client.CustomRoute, upstream client.Upstream, gateway *gatewayv1.Gateway) error {
				Expect(route.GetName()).To(Equal("test-route"))
				Expect(route.GetHost()).To(Equal("gateway1.example.com"))
				Expect(route.GetPath()).To(Equal("/foo"))
				Expect(upstream.GetHost()).To(Equal("localhost"))
				Expect(upstream.GetScheme()).To(Equal("http"))
				Expect(upstream.GetPort()).To(Equal(8080))
				Expect(upstream.GetPath()).To(Equal("/proxy"))
				return nil
			}
			mockKc.EXPECT().CreateOrReplaceRoute(ctx, gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&gatewayv1.Gateway{})).DoAndReturn(mockCreateOrReplaceRoute).Times(1)

			mockCreateOrReplacePlugin := func(ctx context.Context, customPlugin client.CustomPlugin) (kongPlugin *kong.Plugin, err error) {
				switch p := customPlugin.(type) {
				case *plugin.RequestTransformerPlugin:
					Expect(p.GetName()).To(Equal("request-transformer"))
					b64str := p.Config.Append.Headers.Get(plugin.RoutingConfigKey)
					routingCfg, err := plugin.FromBase64[plugin.RoutingConfigs](b64str)
					if err != nil {
						Fail("Failed to decode routing config: " + err.Error())
					}
					Expect(routingCfg).ToNot(BeNil())
					Expect(routingCfg.Len()).To(Equal(2))

					normalUpstream := routingCfg.Get(0)
					Expect(normalUpstream.TargetZoneName).To(Equal("zone1"))
					Expect(normalUpstream.Issuer).To(Equal("https://issuer2.example.com"))
					Expect(normalUpstream.RemoteApiUrl).To(Equal("http://gateway2.example.com:80"))

					failoverUpstream := routingCfg.Get(1)
					Expect(failoverUpstream.TargetZoneName).To(Equal(""))
					Expect(failoverUpstream.Issuer).To(Equal(""))
					Expect(failoverUpstream.RemoteApiUrl).To(Equal(""))
					Expect(failoverUpstream.LoadBalancing).ToNot(BeNil())
					Expect(failoverUpstream.LoadBalancing.Servers).To(HaveCap(2))
					Expect(failoverUpstream.LoadBalancing.Servers[0].Upstream).To(Equal("http://upstream1:80/"))
					Expect(failoverUpstream.LoadBalancing.Servers[0].Weight).To(Equal(50))
					Expect(failoverUpstream.LoadBalancing.Servers[1].Upstream).To(Equal("http://upstream2:80/"))
					Expect(failoverUpstream.LoadBalancing.Servers[1].Weight).To(Equal(50))

				default:
					Fail("Unexpected plugin type: " + p.GetName())
				}

				return nil, nil
			}
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).DoAndReturn(mockCreateOrReplacePlugin).Times(1)

			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

			builder := features.NewFeatureBuilder(mockKc, route, nil, realm, gateway)
			builder.EnableFeature(feature.InstanceFailoverFeature)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Correctly configure failover as proxy-route", func() {

		var ctx context.Context
		var mockCtrl *gomock.Controller

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			ctx = context.Background()
			ctx = contextutil.WithEnv(ctx, "test")
		})

		It("should have the correct priority", func() {
			Expect(feature.InstanceFailoverFeature.Priority()).To(Equal(feature.InstanceLastMileSecurityFeature.Priority() - 1))
		})

		It("should apply failover feature when it is used", func() {
			mockKc := mock.NewMockKongClient(mockCtrl)

			route := NewRoute()
			route.Spec.Downstreams = []gatewayv1.Downstream{
				{
					Host:      "gateway1.example.com",
					Path:      "/foo",
					IssuerUrl: "https://issuer1.example.com",
				},
			}
			route.Spec.Upstreams = []gatewayv1.Upstream{
				{
					Scheme:    "http",
					Host:      "gateway2.example.com",
					Port:      80,
					IssuerUrl: "https://issuer2.example.com",
				},
			}
			route.Spec.Traffic = gatewayv1.Traffic{
				Failover: &gatewayv1.Failover{
					TargetZoneName: "zone1",
					Upstreams: []gatewayv1.Upstream{
						{
							Scheme:    "http",
							Host:      "upstream1",
							Port:      80,
							Path:      "/",
							IssuerUrl: "https://issuer2.example.com",
						},
					},
				},
			}
			realm := NewRealm()
			gateway := NewGateway()

			// Expects
			mockCreateOrReplaceRoute := func(ctx context.Context, route client.CustomRoute, upstream client.Upstream, gateway *gatewayv1.Gateway) error {
				Expect(route.GetName()).To(Equal("test-route"))
				Expect(route.GetHost()).To(Equal("gateway1.example.com"))
				Expect(route.GetPath()).To(Equal("/foo"))
				Expect(upstream.GetHost()).To(Equal("localhost"))
				Expect(upstream.GetScheme()).To(Equal("http"))
				Expect(upstream.GetPort()).To(Equal(8080))
				Expect(upstream.GetPath()).To(Equal("/proxy"))
				return nil
			}
			mockKc.EXPECT().CreateOrReplaceRoute(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(mockCreateOrReplaceRoute).Times(1)

			mockCreateOrReplacePlugin := func(ctx context.Context, customPlugin client.CustomPlugin) (kongPlugin *kong.Plugin, err error) {
				switch p := customPlugin.(type) {
				case *plugin.RequestTransformerPlugin:
					Expect(p.GetName()).To(Equal("request-transformer"))
					b64str := p.Config.Append.Headers.Get(plugin.RoutingConfigKey)
					routingCfg, err := plugin.FromBase64[plugin.RoutingConfigs](b64str)
					if err != nil {
						Fail("Failed to decode routing config: " + err.Error())
					}
					Expect(routingCfg).ToNot(BeNil())
					Expect(routingCfg.Len()).To(Equal(2))

					normalUpstream := routingCfg.Get(0)
					Expect(normalUpstream.TargetZoneName).To(Equal("zone1"))
					Expect(normalUpstream.Issuer).To(Equal("https://issuer2.example.com"))
					Expect(normalUpstream.RemoteApiUrl).To(Equal("http://gateway2.example.com:80"))

					failoverUpstream := routingCfg.Get(1)
					Expect(failoverUpstream.TargetZoneName).To(Equal(""))
					Expect(failoverUpstream.Issuer).To(Equal("https://issuer2.example.com"))
					Expect(failoverUpstream.RemoteApiUrl).To(Equal("http://upstream1:80/"))

				default:
					Fail("Unexpected plugin type: " + p.GetName())
				}

				return nil, nil
			}
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).DoAndReturn(mockCreateOrReplacePlugin).Times(1)

			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

			builder := features.NewFeatureBuilder(mockKc, route, nil, realm, gateway)
			builder.EnableFeature(feature.InstanceFailoverFeature)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
