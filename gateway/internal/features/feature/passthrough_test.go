// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

var _ = Describe("PassThroughFeature", func() {

	var (
		ctx     context.Context
		f       *feature.PassThroughFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstancePassThroughFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypePassThrough", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypePassThrough))
		})
	})

	Describe("Priority()", func() {
		It("returns 0", func() {
			Expect(f.Priority()).To(Equal(0))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route is passthrough with upstreams", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "example.com", Port: 443, Path: "/api"},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route is not passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{
								{Scheme: "https", Hostname: "example.com", Port: 443, Path: "/api"},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route has no upstreams", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{},
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
			It("sets upstream to the first backend upstream", func() {
				upstream := gatewayv1.Upstream{
					Scheme:   "https",
					Hostname: "backend.example.com",
					Port:     8443,
					Path:     "/v1",
				}
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Backend: gatewayv1.Backend{
							Upstreams: []gatewayv1.Upstream{upstream},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) {
					Expect(u.GetScheme()).To(Equal("https"))
					Expect(u.GetHostname()).To(Equal("backend.example.com"))
					Expect(u.GetPort()).To(Equal(8443))
					Expect(u.GetPath()).To(Equal("/v1"))
				}).Return()

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())
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
	})
})
