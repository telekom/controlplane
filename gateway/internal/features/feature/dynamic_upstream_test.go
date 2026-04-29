// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ = Describe("DynamicUpstreamFeature", func() {

	It("should return the correct feature type", func() {
		Expect(InstanceDynamicUpstreamFeature.Name()).To(Equal(gatewayv1.FeatureTypeDynamicUpstream))
	})

	It("should have priority higher than LastMileSecurityFeature", func() {
		Expect(InstanceDynamicUpstreamFeature.Priority()).To(Equal(InstanceLastMileSecurityFeature.Priority() + 1))
	})

	Context("with mocked feature builder", func() {

		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder
		var feature DynamicUpstreamFeature

		BeforeEach(func() {
			feature = *InstanceDynamicUpstreamFeature

			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(GinkgoT())
		})

		Context("check IsUsed", func() {
			It("should return false when no route in builder", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return false when route has no upstreams", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return false when route has multiple upstreams", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "localhost"},
							{Host: "localhost"},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return false when upstream is a proxy", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{
								Host:      "localhost",
								IssuerUrl: "https://issuer.example.com",
							},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return false when upstream host is not localhost", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "example.com"},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return false when no DynamicUpstream config", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "localhost"},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should return true when all conditions are met", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "localhost"},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})
		})

		Context("Apply", func() {
			It("should return ErrNoRoute when no route in builder", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false)
				err := feature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).To(MatchError(features.ErrNoRoute))
			})

			It("should replace static remote_api_url with dynamic value", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "localhost"},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "target_url",
							},
						},
					},
				}

				rtpPlugin := &plugin.RequestTransformerPlugin{
					Config: plugin.RequestTransformerPluginConfig{},
				}
				// Simulate LastMileSecurity having set the static header
				rtpPlugin.Config.Append.AddHeader("remote_api_url", "https://static.example.com")

				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

				err := feature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).ToNot(HaveOccurred())

				// Verify static header was replaced with dynamic value
				Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("$(query_params.target_url)"))
				// Verify query parameter is removed from forwarded request
				Expect(rtpPlugin.Config.Remove.Querystring).ToNot(BeNil())
				Expect(rtpPlugin.Config.Remove.Querystring.Contains("target_url")).To(BeTrue())
			})

			It("should use the configured query parameter name", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{Host: "localhost"},
						},
						Traffic: gatewayv1.Traffic{
							DynamicUpstream: &gatewayv1.DynamicUpstream{
								QueryParameter: "api_endpoint",
							},
						},
					},
				}

				rtpPlugin := &plugin.RequestTransformerPlugin{
					Config: plugin.RequestTransformerPluginConfig{},
				}
				rtpPlugin.Config.Append.AddHeader("remote_api_url", "https://original.example.com")

				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true)
				mockFeatureBuilder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

				err := feature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).ToNot(HaveOccurred())

				Expect(rtpPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("$(query_params.api_endpoint)"))
				Expect(rtpPlugin.Config.Remove.Querystring).ToNot(BeNil())
				Expect(rtpPlugin.Config.Remove.Querystring.Contains("api_endpoint")).To(BeTrue())
			})
		})
	})
})
