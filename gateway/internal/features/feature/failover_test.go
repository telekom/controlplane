// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FailoverFeature", func() {

	var (
		ctx     context.Context
		f       *feature.FailoverFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		f = feature.InstanceFailoverFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeFailover", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeFailover))
		})
	})

	Describe("Priority()", func() {
		It("returns 109", func() {
			Expect(f.Priority()).To(Equal(109))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route has failover with upstreams", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								TargetZoneName: "zone-a",
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "failover.example.com", Port: 443, Path: "/v1"},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has failover with empty upstreams", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								TargetZoneName: "zone-a",
								Upstreams:      []gatewayv1.Upstream{},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route has no failover", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{},
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
			Context("when failover secondary with single upstream", func() {
				It("creates 2 routing configs, second has failover URL", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secondary-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeSecondary,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "primary.example.com", Port: 443, Path: "/api"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-a",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "failover.example.com", Port: 8443, Path: "/v1"},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Two routing configs should be added
					Expect(routingConfigs.Len()).To(Equal(2))

					// First routing config: proxy to primary upstream
					proxyConfig := routingConfigs.Get(0)
					Expect(proxyConfig.RemoteApiUrl).To(Equal("https://primary.example.com:443/api"))
					Expect(proxyConfig.ApiBasePath).To(Equal("/api"))
					Expect(proxyConfig.Realm).To(Equal("test-realm"))
					Expect(proxyConfig.Environment).To(Equal("test-env"))
					Expect(proxyConfig.TargetZoneName).To(Equal("zone-a"))
					Expect(proxyConfig.JumperConfig).To(BeNil())

					// Second routing config: failover upstream
					failoverConfig := routingConfigs.Get(1)
					Expect(failoverConfig.RemoteApiUrl).To(Equal("https://failover.example.com:8443/v1"))
					Expect(failoverConfig.ApiBasePath).To(Equal("/v1"))
					Expect(failoverConfig.Realm).To(Equal("test-realm"))
					Expect(failoverConfig.Environment).To(Equal("test-env"))
					Expect(failoverConfig.JumperConfig).To(Equal(jumperConfig))
				})
			})

			Context("when failover secondary with multiple upstreams (load balancing)", func() {
				It("second routing config has LoadBalancing servers", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secondary-lb-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeSecondary,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "primary.example.com", Port: 443, Path: "/api"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-a",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "failover-a.example.com", Port: 443, Path: "/v1", Weight: 70},
										{Scheme: "https", Hostname: "failover-b.example.com", Port: 443, Path: "/v1", Weight: 30},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(routingConfigs.Len()).To(Equal(2))

					// Second routing config should have LoadBalancing set
					failoverConfig := routingConfigs.Get(1)
					Expect(failoverConfig.LoadBalancing).ToNot(BeNil())
					Expect(failoverConfig.LoadBalancing.Servers).To(HaveLen(2))
					Expect(failoverConfig.LoadBalancing.Servers[0].Upstream).To(Equal("https://failover-a.example.com:443/v1"))
					Expect(failoverConfig.LoadBalancing.Servers[0].Weight).To(Equal(int32(70)))
					Expect(failoverConfig.LoadBalancing.Servers[1].Upstream).To(Equal("https://failover-b.example.com:443/v1"))
					Expect(failoverConfig.LoadBalancing.Servers[1].Weight).To(Equal(int32(30)))
					// RemoteApiUrl should NOT be set when LB is used
					Expect(failoverConfig.RemoteApiUrl).To(BeEmpty())
				})
			})

			Context("when proxy route (not secondary) with single failover upstream", func() {
				It("second routing config has failover URL and empty TargetZoneName", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "proxy-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeProxy,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "primary.example.com", Port: 443, Path: "/api"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-a",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "secondary.example.com", Port: 443, Path: "/v2"},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(routingConfigs.Len()).To(Equal(2))

					// Second routing config: failover upstream with empty TargetZoneName
					failoverConfig := routingConfigs.Get(1)
					Expect(failoverConfig.RemoteApiUrl).To(Equal("https://secondary.example.com:443/v2"))
					Expect(failoverConfig.ApiBasePath).To(Equal("/v2"))
					Expect(failoverConfig.TargetZoneName).To(BeEmpty())
				})
			})

			Context("when failover secondary with ExternalIDP security", func() {
				It("removes token_endpoint header and sets TokenEndpoint in routing config", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "secondary-idp-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeSecondary,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "primary.example.com", Port: 443, Path: "/api"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-a",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "failover.example.com", Port: 443, Path: "/v1"},
									},
									Security: gatewayv1.Security{
										M2M: &gatewayv1.Machine2MachineAuthentication{
											ExternalIDP: &gatewayv1.ExternalIdentityProvider{
												TokenEndpoint: "https://failover-idp.example.com/token",
											},
										},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					// Create an RTP plugin with token_endpoint pre-added (simulating ExternalIDP feature)
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					rtpPlugin.Config.Append.AddHeader("token_endpoint", "https://primary-idp.example.com/token")

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// The failover routing config should have TokenEndpoint set
					failoverConfig := routingConfigs.Get(1)
					Expect(failoverConfig.TokenEndpoint).To(Equal("https://failover-idp.example.com/token"))

					// The token_endpoint header should be removed from RTP
					Expect(rtpPlugin.Config.Append.Headers.Contains("token_endpoint")).To(BeFalse())
				})
			})
		})

		Context("error handling", func() {
			Context("when no route in builder", func() {
				It("returns ErrNoRoute", func() {
					routingConfigs := &plugin.RoutingConfigs{}
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().GetRoute().Return(nil, false)

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoRoute))
				})
			})

			Context("when proxy route (not secondary) with multiple upstreams", func() {
				It("returns error about unsupported loadbalancing", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "proxy-lb-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeProxy,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "primary.example.com", Port: 443, Path: "/api"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-a",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "failover-a.example.com", Port: 443, Path: "/v1", Weight: 50},
										{Scheme: "https", Hostname: "failover-b.example.com", Port: 443, Path: "/v1", Weight: 50},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("loadbalancing is not supported for proxy routes that are not failover secondary routes"))
				})
			})
		})

		Context("edge cases", func() {
			Context("when upstream URL is correctly constructed from scheme/hostname/port/path", func() {
				It("produces the expected URL format", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "url-format-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeSecondary,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "http", Hostname: "backend.internal", Port: 8080, Path: "/service/v3"},
								},
							},
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-west",
									Upstreams: []gatewayv1.Upstream{
										{Scheme: "https", Hostname: "failover.external.io", Port: 9443, Path: "/api/v2"},
									},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "my-realm",
							},
						},
					}

					routingConfigs := &plugin.RoutingConfigs{}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RoutingConfigs().Return(routingConfigs)
					builder.EXPECT().SetUpstream(mock.Anything).Return()
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Verify primary upstream URL format
					proxyConfig := routingConfigs.Get(0)
					Expect(proxyConfig.RemoteApiUrl).To(Equal("http://backend.internal:8080/service/v3"))
					Expect(proxyConfig.ApiBasePath).To(Equal("/service/v3"))

					// Verify failover upstream URL format
					failoverConfig := routingConfigs.Get(1)
					Expect(failoverConfig.RemoteApiUrl).To(Equal("https://failover.external.io:9443/api/v2"))
					Expect(failoverConfig.ApiBasePath).To(Equal("/api/v2"))
				})
			})
		})
	})
})
