// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ = Describe("ClaimsFeature", func() {

	var (
		ctx     context.Context
		f       *feature.ClaimsFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceClaimsFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeClaims", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeClaims))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		It("returns true for a primary, non-passthrough route", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{Type: gatewayv1.RouteTypePrimary},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeTrue())
		})

		It("returns false for a proxy route", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{Type: gatewayv1.RouteTypeProxy},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("returns false when no route in builder", func() {
			builder.EXPECT().GetRoute().Return(nil, false)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})
	})

	Describe("Apply()", func() {
		It("writes provider exposure claims into the default bucket", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{
								{Key: "aud", Value: "eni--foo--api-provider-rover"},
							},
						},
					},
				},
			}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

			Expect(f.Apply(ctx, builder)).To(Succeed())

			def := jumperConfig.Claims[plugin.ConsumerId(feature.DefaultProviderKey)]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Key).To(Equal("aud"))
			Expect(def[0].Value).To(Equal("eni--foo--api-provider-rover"))
			Expect(def[0].ValueFrom).To(BeEmpty())
		})

		It("keeps a symbolic ConsumerClientId claim in the default bucket", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{
								{Key: "aud", ValueFrom: gatewayv1.ClaimValueFromConsumerClientId},
							},
						},
					},
				},
			}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

			Expect(f.Apply(ctx, builder)).To(Succeed())

			def := jumperConfig.Claims[plugin.ConsumerId(feature.DefaultProviderKey)]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Value).To(BeEmpty())
			Expect(def[0].ValueFrom).To(Equal("ConsumerClientId"))
		})

		It("writes per-consumer overrides under the consumer key", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{Security: gatewayv1.Security{}}}
			consumers := []*gatewayv1.ConsumeRoute{
				{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "eni--bar--consumer-a",
						Security: &gatewayv1.ConsumeRouteSecurity{
							M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
								Claims: []gatewayv1.Claim{{Key: "aud", Value: "custom-for-consumer-a"}},
							},
						},
					},
				},
			}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return(consumers)

			Expect(f.Apply(ctx, builder)).To(Succeed())

			c := jumperConfig.Claims[plugin.ConsumerId("eni--bar--consumer-a")]
			Expect(c).To(HaveLen(1))
			Expect(c[0].Value).To(Equal("custom-for-consumer-a"))
		})

		It("leaves Claims empty when neither route nor consumers have claims", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{Security: gatewayv1.Security{}}}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

			Expect(f.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.Claims).To(BeEmpty())
		})

		It("skips claims when OAuth is already populated (external IDP owns the token)", func() {
			jumperConfig := plugin.NewJumperConfig()
			jumperConfig.OAuth[plugin.ConsumerId(feature.DefaultProviderKey)] = plugin.OauthCredentials{Scopes: "scope-a"}
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{{Key: "aud", Value: "ignored"}},
						},
					},
				},
			}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.Claims).To(BeEmpty())
		})

		It("returns ErrNoRoute when no route in builder", func() {
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(nil, false)

			Expect(f.Apply(ctx, builder)).To(MatchError(features.ErrNoRoute))
		})
	})
})
