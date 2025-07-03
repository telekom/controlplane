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
	"go.uber.org/mock/gomock"
)

var _ = Describe("ExternalIDPFeature", func() {

	It("should have a lower priority than CustomScopesFeature", func() {
		Expect(InstanceExternalIDPFeature.Priority()).To(BeNumerically("<", InstanceCustomScopesFeature.Priority()))
	})

	It("should return the correct feature type", func() {
		Expect(InstanceExternalIDPFeature.Name()).To(Equal(gatewayv1.FeatureTypeExternalIDP))
	})

	It("should return upstream if external idp is configured", func() {
		upstreams := []gatewayv1.Upstream{
			{
				Scheme:    "https",
				Host:      "example.com",
				Port:      0,
				Path:      "/api",
				IssuerUrl: "example.com/issuer",
				Security:  nil,
			},
			{
				Scheme:    "https",
				Host:      "example.com",
				Port:      0,
				Path:      "/api",
				IssuerUrl: "example.com/issuer",
				Security: &gatewayv1.Security{
					M2M: &gatewayv1.Machine2MachineAuthentication{
						ExternalIDPConfig: &gatewayv1.ExternalIdentityProviderConfig{
							TokenEndpoint: "example.com/tokenEndpoint",
						},
					},
				},
			},
		}
		upstream := getFirstUpstreamWithIDPConfig(&upstreams)
		Expect(upstream).ToNot(BeNil())
		Expect(upstream.Security.M2M.ExternalIDPConfig.TokenEndpoint).To(Equal("example.com/tokenEndpoint"))
	})

	It("should return nil if no external idp is no is present", func() {
		upstreams := []gatewayv1.Upstream{
			{
				Scheme:    "https",
				Host:      "example.com",
				Port:      0,
				Path:      "/api",
				IssuerUrl: "example.com/issuer",
				Security:  nil,
			},
			{
				Scheme:    "https",
				Host:      "example.com",
				Port:      0,
				Path:      "/api",
				IssuerUrl: "example.com/issuer",
				Security: &gatewayv1.Security{
					M2M: nil,
				},
			},
			{
				Scheme:    "https",
				Host:      "example.com",
				Port:      0,
				Path:      "/api",
				IssuerUrl: "example.com/issuer",
				Security: &gatewayv1.Security{
					M2M: &gatewayv1.Machine2MachineAuthentication{
						ExternalIDPConfig: nil,
					},
				},
			},
		}
		upstream := getFirstUpstreamWithIDPConfig(&upstreams)
		Expect(upstream).To(BeNil())
	})

	Context("with mocked feature builder", func() {

		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder
		var feature ExternalIDPFeature

		BeforeEach(func() {
			feature = ExternalIDPFeature{}

			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
		})

		Context("check IsUsed", func() {
			It("nil route, external IDP should not be used", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("route with no upstreams, external IDP should not be used", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("proxy route, external IDP should not be used", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Upstreams: []gatewayv1.Upstream{
							{
								Scheme:    "https",
								Host:      "example.com",
								Port:      0,
								Path:      "/api",
								IssuerUrl: "example.com/issuer", //has IssuerUrl == Proxy Route
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("route with upstreams with idp config, external IDP should be used", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{
								Scheme:    "https",
								Host:      "example.com",
								Port:      0,
								Path:      "/api",
								IssuerUrl: "", // no Issuer == Real Route
								Security: &gatewayv1.Security{
									M2M: &gatewayv1.Machine2MachineAuthentication{
										ExternalIDPConfig: &gatewayv1.ExternalIdentityProviderConfig{
											TokenEndpoint: "example.com/tokenEndpoint",
										},
									},
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})
		})
	})
})
