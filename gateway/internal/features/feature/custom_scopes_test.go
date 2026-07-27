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

var _ = Describe("CustomScopesFeature", func() {

	var (
		ctx     context.Context
		f       *feature.CustomScopesFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceCustomScopesFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeCustomScopes", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeCustomScopes))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route is primary and not passthrough", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: false,
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route is failover secondary and not passthrough", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypeSecondary,
						PassThrough: false,
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
						Type:        gatewayv1.RouteTypeProxy,
						PassThrough: false,
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route is passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: true,
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
			Context("when route has M2M scopes", func() {
				It("populates JumperConfig OAuth for default provider with space-joined scopes", func() {
					jumperConfig := plugin.NewJumperConfig()
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Scopes: []string{"read", "write", "admin"},
								},
							},
						},
					}

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId(feature.DefaultProviderKey)))
					creds := jumperConfig.OAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
					Expect(creds.Scopes).To(Equal("read write admin"))
				})
			})

			Context("when consumers have M2M scopes", func() {
				It("populates JumperConfig OAuth per consumer with space-joined scopes", func() {
					jumperConfig := plugin.NewJumperConfig()
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{},
						},
					}
					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-a",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Scopes: []string{"scope1", "scope2"},
									},
								},
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-b",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Scopes: []string{"scopeX"},
									},
								},
							},
						},
					}

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId("consumer-a")))
					Expect(jumperConfig.OAuth[plugin.ConsumerId("consumer-a")].Scopes).To(Equal("scope1 scope2"))
					Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId("consumer-b")))
					Expect(jumperConfig.OAuth[plugin.ConsumerId("consumer-b")].Scopes).To(Equal("scopeX"))
				})
			})
		})

		Context("error handling", func() {
			Context("when no route in builder", func() {
				It("returns ErrNoRoute", func() {
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(nil, false)

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoRoute))
				})
			})
		})

		Context("edge cases", func() {
			Context("when JumperConfig OAuth is already populated", func() {
				It("returns immediately without modifying OAuth", func() {
					jumperConfig := plugin.NewJumperConfig()
					jumperConfig.OAuth[plugin.ConsumerId("existing")] = plugin.OauthCredentials{
						Scopes: "pre-existing-scope",
					}

					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Scopes: []string{"should-not-appear"},
								},
							},
						},
					}

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(route, true)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.OAuth).To(HaveLen(1))
					Expect(jumperConfig.OAuth[plugin.ConsumerId("existing")].Scopes).To(Equal("pre-existing-scope"))
				})
			})

			Context("when no M2M on route or consumers", func() {
				It("leaves JumperConfig OAuth empty", func() {
					jumperConfig := plugin.NewJumperConfig()
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{},
						},
					}
					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-no-m2m",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: nil,
								},
							},
						},
					}

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.OAuth).To(BeEmpty())
				})
			})

			Context("when consumer has nil security", func() {
				It("skips that consumer", func() {
					jumperConfig := plugin.NewJumperConfig()
					route := &gatewayv1.Route{
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{},
						},
					}
					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-nil-security",
								Security:     nil,
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-with-scopes",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Scopes: []string{"valid-scope"},
									},
								},
							},
						},
					}

					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(jumperConfig.OAuth).ToNot(HaveKey(plugin.ConsumerId("consumer-nil-security")))
					Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId("consumer-with-scopes")))
					Expect(jumperConfig.OAuth[plugin.ConsumerId("consumer-with-scopes")].Scopes).To(Equal("valid-scope"))
				})
			})
		})
	})
})
