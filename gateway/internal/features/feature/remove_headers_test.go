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

var _ = Describe("RemoveHeadersFeature", func() {

	It("should return the correct feature type", func() {
		Expect(InstanceRemoveHeadersFeature.Name()).To(Equal(gatewayv1.FeatureTypeRemoveHeaders))
	})

	Context("with mocked feature builder", func() {

		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder
		var feature RemoveHeadersFeature

		BeforeEach(func() {
			feature = RemoveHeadersFeature{}

			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
		})

		Context("check IsUsed", func() {
			It("proxy route, RemoveHeaders should not be used", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{
								Scheme:    "https",
								Host:      "example.com",
								Port:      0,
								Path:      "/api",
								IssuerUrl: "http://issuer", // Issuer == Proxy Route
							},
						},
						Transformation: &gatewayv1.Transformation{
							Request: gatewayv1.RequestResponseTransformation{
								Headers: gatewayv1.HeaderTransformation{
									Remove: []string{"X-Remove-Header"},
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route)
				Expect(feature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("real route, RemoveHeaders should be used", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Upstreams: []gatewayv1.Upstream{
							{
								Scheme:    "https",
								Host:      "example.com",
								Port:      0,
								Path:      "/api",
								IssuerUrl: "", // no Issuer == Real Route
							},
						},
						Transformation: &gatewayv1.Transformation{
							Request: gatewayv1.RequestResponseTransformation{
								Headers: gatewayv1.HeaderTransformation{
									Remove: []string{"X-Remove-Header"},
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
