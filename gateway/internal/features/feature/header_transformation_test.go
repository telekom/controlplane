// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HeaderTransformationFeature", func() {
	var (
		ctx     context.Context
		f       *feature.HeaderTransformationFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceHeaderTransformationFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeHeaderTransformation", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeHeaderTransformation))
		})
	})

	Describe("Priority()", func() {
		It("returns 0", func() {
			Expect(f.Priority()).To(Equal(0))
		})
	})

	Describe("IsUsed()", func() {
		Context("when primary route has transformation spec", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Transformation: &gatewayv1.Transformation{
							Request: gatewayv1.RequestResponseTransformation{
								Headers: gatewayv1.HeaderTransformation{
									Remove: []string{"X-Custom-Header"},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route is proxy", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypeProxy,
						Transformation: &gatewayv1.Transformation{
							Request: gatewayv1.RequestResponseTransformation{
								Headers: gatewayv1.HeaderTransformation{
									Remove: []string{"X-Custom-Header"},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no transformation spec", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:           gatewayv1.RouteTypePrimary,
						Transformation: nil,
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
			Context("when transformation has headers to remove", func() {
				It("adds each header to RTP Config.Remove", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Transformation: &gatewayv1.Transformation{
								Request: gatewayv1.RequestResponseTransformation{
									Headers: gatewayv1.HeaderTransformation{
										Remove: []string{"X-Custom-Header", "X-Internal-Token"},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(rtpPlugin.Config.Remove.Headers.Contains("X-Custom-Header")).To(BeTrue())
					Expect(rtpPlugin.Config.Remove.Headers.Contains("X-Internal-Token")).To(BeTrue())
					Expect(rtpPlugin.Config.Remove.Headers.Size()).To(Equal(2))
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
			Context("when remove list is empty", func() {
				It("does not add any headers to the plugin", func() {
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypePrimary,
							Transformation: &gatewayv1.Transformation{
								Request: gatewayv1.RequestResponseTransformation{
									Headers: gatewayv1.HeaderTransformation{
										Remove: []string{},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(rtpPlugin.Config.Remove.Headers).To(BeNil())
				})
			})
		})
	})
})
