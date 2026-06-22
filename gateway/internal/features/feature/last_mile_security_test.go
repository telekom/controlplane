// Copyright 2025 Deutsche Telekom IT GmbH
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
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ = Describe("LastMileSecurityFeature", func() {

	var (
		ctx     context.Context
		f       *feature.LastMileSecurityFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		f = feature.InstanceLastMileSecurityFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeLastMileSecurity", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeLastMileSecurity))
		})
	})

	Describe("Priority()", func() {
		It("returns 100", func() {
			Expect(f.Priority()).To(Equal(100))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route is not passthrough and has no failover", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							Failover: nil,
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route is passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Traffic: gatewayv1.Traffic{
							Failover: nil,
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route has failover configured", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								TargetZoneName: "zone-b",
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
			Context("when route is a primary (real) route", func() {
				It("sets upstream to localhost, adds env/realm, removes consumer-token, adds remote_api_url/api_base_path/access_token_forwarding", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "api.example.com", Port: 443, Path: "/v1"},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) {
						Expect(u.GetScheme()).To(Equal("http"))
						Expect(u.GetHostname()).To(Equal("localhost"))
						Expect(u.GetPort()).To(Equal(8080))
						Expect(u.GetPath()).To(Equal("/proxy"))
					}).Return()

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Add headers: environment and realm
					Expect(rtpPlugin.Config.Add.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Add.Headers.Get("environment")).To(Equal("test-env"))
					Expect(rtpPlugin.Config.Add.Headers.Get("realm")).To(Equal("test-realm"))

					// Replace headers: environment, realm, and Authorization template
					Expect(rtpPlugin.Config.Replace.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Replace.Headers.Get("environment")).To(Equal("test-env"))
					Expect(rtpPlugin.Config.Replace.Headers.Get("realm")).To(Equal("test-realm"))
					Expect(rtpPlugin.Config.Replace.Headers.Get("Authorization")).To(
						Equal("$(headers['consumer-token'] or headers['Authorization'])"))

					// Remove headers: consumer-token
					Expect(rtpPlugin.Config.Remove.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Remove.Headers.Contains("consumer-token")).To(BeTrue())

					// Append headers: remote_api_url, api_base_path, access_token_forwarding
					Expect(rtpPlugin.Config.Append.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("https://api.example.com:443/v1"))
					Expect(rtpPlugin.Config.Append.Headers.Get("api_base_path")).To(Equal("/v1"))
					Expect(rtpPlugin.Config.Append.Headers.Get("access_token_forwarding")).To(Equal("false"))
				})
			})

			Context("when route is a proxy route", func() {
				It("sets upstream to localhost, adds remote_api_url and mock-issuer, does NOT remove consumer-token", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeProxy,
							Backend: gatewayv1.Backend{
								Upstreams: []gatewayv1.Upstream{
									{Scheme: "https", Hostname: "proxy.example.com", Port: 8443, Path: "/api"},
								},
							},
							Security: gatewayv1.Security{
								RealmName: "proxy-realm",
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().SetUpstream(mock.Anything).Return()

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Add headers: environment and realm
					Expect(rtpPlugin.Config.Add.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Add.Headers.Get("environment")).To(Equal("test-env"))
					Expect(rtpPlugin.Config.Add.Headers.Get("realm")).To(Equal("proxy-realm"))

					// Replace headers: environment and realm only (no Authorization template)
					Expect(rtpPlugin.Config.Replace.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Replace.Headers.Get("environment")).To(Equal("test-env"))
					Expect(rtpPlugin.Config.Replace.Headers.Get("realm")).To(Equal("proxy-realm"))
					Expect(rtpPlugin.Config.Replace.Headers.Contains("Authorization")).To(BeFalse())

					// Remove: consumer-token is NOT removed for proxy routes
					Expect(rtpPlugin.Config.Remove.Headers).To(BeNil())

					// Append headers: remote_api_url and issuer (proxy-specific)
					Expect(rtpPlugin.Config.Append.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("https://proxy.example.com:8443/api"))
					Expect(rtpPlugin.Config.Append.Headers.Get("issuer")).To(Equal("mock-issuer"))

					// Proxy route does NOT set api_base_path or access_token_forwarding
					Expect(rtpPlugin.Config.Append.Headers.Contains("api_base_path")).To(BeFalse())
					Expect(rtpPlugin.Config.Append.Headers.Contains("access_token_forwarding")).To(BeFalse())
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

		Describe("CreateRemoteApiUrl()", func() {
			It("constructs URL with scheme, hostname, port, and path", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api.example.com", Port: 443, Path: "/v1"},
							},
						},
					},
				}

				Expect(feature.CreateRemoteApiUrl(route)).To(Equal("https://api.example.com:443/v1"))
			})

			It("omits port when port is 0", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api.example.com", Port: 0, Path: "/v1"},
							},
						},
					},
				}

				Expect(feature.CreateRemoteApiUrl(route)).To(Equal("https://api.example.com/v1"))
			})

			It("collapses double slashes in path", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "api.example.com", Port: 0, Path: "//v1//api"},
							},
						},
					},
				}

				Expect(feature.CreateRemoteApiUrl(route)).To(Equal("https://api.example.com/v1/api"))
			})
		})
	})
})
