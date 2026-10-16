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
		It("returns true for a primary, non-passthrough route with claims", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{{Key: "aud", Value: "some-aud"}},
						},
					},
				},
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

		It("returns false when an external IDP owns the token", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							ExternalIDP: &gatewayv1.ExternalIdentityProvider{},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("returns false when basic auth owns the token", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Basic: &gatewayv1.BasicAuthCredentials{},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("returns false when no route in builder", func() {
			builder.EXPECT().GetRoute().Return(nil, false)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})
	})

	Describe("failover routes", func() {
		It("IsUsed uses failover security and returns false when external IDP owns it", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypeSecondary,
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("IsUsed returns true for a failover route whose failover security owns the LMS token", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypeSecondary,
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Claims: []gatewayv1.Claim{{Key: "aud", Value: "failover-aud"}},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeTrue())
		})

		It("IsUsed returns false when basic auth owns the failover token", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypeSecondary,
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Basic: &gatewayv1.BasicAuthCredentials{},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("IsUsed returns false for a passthrough failover route even with claims configured", func() {
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type:        gatewayv1.RouteTypeSecondary,
					PassThrough: true,
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Claims: []gatewayv1.Claim{{Key: "aud", Value: "failover-aud"}},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().GetRoute().Return(route, true)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("Apply prefers failover security over Spec.Security when both carry claims", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypeSecondary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{{Key: "aud", Value: "spec-aud"}},
						},
					},
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Claims: []gatewayv1.Claim{{Key: "aud", Value: "failover-aud"}},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			def := jumperConfig.Claims[feature.DefaultProviderKey]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Value).To(Equal("failover-aud"))
		})

		It("Apply writes claims from failover security, not Spec.Security", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type:     gatewayv1.RouteTypeSecondary,
					Security: gatewayv1.Security{},
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Claims: []gatewayv1.Claim{{Key: "aud", Value: "failover-aud"}},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			def := jumperConfig.Claims[feature.DefaultProviderKey]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Value).To(Equal("failover-aud"))
		})

		// A primary route uses its own Spec.Security. Failover config only
		// applies to secondary routes, so a primary route never resolves via
		// Traffic.Failover.Security.
		It("Apply uses Spec.Security on a primary route regardless of failover config", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{{Key: "aud", Value: "spec-aud"}},
						},
					},
					Traffic: gatewayv1.Traffic{
						Failover: &gatewayv1.Failover{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Claims: []gatewayv1.Claim{{Key: "aud", Value: "failover-aud"}},
								},
							},
						},
					},
				},
			}
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			def := jumperConfig.Claims[feature.DefaultProviderKey]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Value).To(Equal("spec-aud"))
		})
	})

	Describe("Apply()", func() {
		It("writes provider exposure claims into the default bucket", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
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

			Expect(f.Apply(ctx, builder)).To(Succeed())

			def := jumperConfig.Claims[feature.DefaultProviderKey]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Key).To(Equal("aud"))
			Expect(def[0].Value).To(Equal("eni--foo--api-provider-rover"))
			Expect(def[0].ValueFrom).To(BeEmpty())
		})

		It("keeps a symbolic ConsumerClientId claim in the default bucket", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
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

			Expect(f.Apply(ctx, builder)).To(Succeed())

			def := jumperConfig.Claims[feature.DefaultProviderKey]
			Expect(def).To(HaveLen(1))
			Expect(def[0].Value).To(BeEmpty())
			Expect(def[0].ValueFrom).To(Equal("ConsumerClientId"))
		})

		It("leaves Claims empty when the route has no claims", func() {
			jumperConfig := plugin.NewJumperConfig()
			route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{Security: gatewayv1.Security{}}}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.Claims).To(BeEmpty())
		})

		It("applies claims even when OAuth is populated by scopes (platform-managed token)", func() {
			jumperConfig := plugin.NewJumperConfig()
			jumperConfig.OAuth[feature.DefaultProviderKey] = plugin.OauthCredentials{Scopes: "scope-a"}
			route := &gatewayv1.Route{
				Spec: gatewayv1.RouteSpec{
					Type: gatewayv1.RouteTypePrimary,
					Security: gatewayv1.Security{
						M2M: &gatewayv1.Machine2MachineAuthentication{
							Claims: []gatewayv1.Claim{{Key: "aud", Value: "applied"}},
						},
					},
				},
			}

			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.Claims).To(HaveKey(feature.DefaultProviderKey))
		})

		It("returns ErrNoRoute when no route in builder", func() {
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetRoute().Return(nil, false)

			Expect(f.Apply(ctx, builder)).To(MatchError(features.ErrNoRoute))
		})
	})
})
