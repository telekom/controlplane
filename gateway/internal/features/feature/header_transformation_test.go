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
				It("sets Jumper removeHeaders without changing other configuration or duplicating headers on reapplication", func() {
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
					jumperConfig := plugin.NewJumperConfig()
					jumperConfig.OAuth["consumer"] = plugin.OauthCredentials{ClientId: "client"}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.RemoveHeaders).To(Equal([]string{"X-Custom-Header", "X-Internal-Token"}))
					Expect(jumperConfig.OAuth["consumer"]).To(Equal(plugin.OauthCredentials{ClientId: "client"}))

					Expect(f.Apply(ctx, builder)).To(Succeed())
					Expect(jumperConfig.RemoveHeaders).To(Equal([]string{"X-Custom-Header", "X-Internal-Token"}))
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
				It("does not initialize Jumper configuration or access the request transformer", func() {
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
					builder.EXPECT().GetRoute().Return(route, true)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when transformation is nil", func() {
				It("does not initialize Jumper configuration or access the request transformer", func() {
					route := &gatewayv1.Route{}
					builder.EXPECT().GetRoute().Return(route, true)

					Expect(f.Apply(ctx, builder)).To(Succeed())
				})
			})
		})
	})
})
