// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/kong/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BasicAuthFeature", func() {

	var (
		ctx     context.Context
		f       *feature.BasicAuthFeature
		builder *featmock.MockKongFeatureBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceBasicAuthFeature
		builder = featmock.NewMockKongFeatureBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeBasicAuth", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeBasicAuth))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		Context("when primary route has basic auth in security", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: false,
						Traffic:     gatewayv1.Traffic{},
						Security: gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "user",
									Password: "pass",
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has failover security with basic auth", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypeProxy,
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								TargetZoneName: "zone-b",
								Security: gatewayv1.Security{
									M2M: &gatewayv1.Machine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "failover-user",
											Password: "failover-pass",
										},
									},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when consumer has basic auth", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: false,
						Traffic:     gatewayv1.Traffic{},
						Security:    gatewayv1.Security{},
					},
				}
				consumers := []*gatewayv1.ConsumeRoute{
					{
						Spec: gatewayv1.ConsumeRouteSpec{
							ConsumerName: "consumer-a",
							Security: &gatewayv1.ConsumeRouteSecurity{
								M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
									Basic: &gatewayv1.BasicAuthCredentials{
										Username: "consumer-user",
										Password: "consumer-pass",
									},
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetAllowedConsumers().Return(consumers)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route is passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: true,
						Traffic:     gatewayv1.Traffic{},
						Security: gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "user",
									Password: "pass",
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route is proxy type (no basic auth check on consumers)", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypeProxy,
						PassThrough: false,
						Traffic:     gatewayv1.Traffic{},
						Security:    gatewayv1.Security{},
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
			Context("when route has basic auth configured", func() {
				It("populates default key in BasicAuth map", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{},
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Basic: &gatewayv1.BasicAuthCredentials{
										Username: "route-user",
										Password: "route-pass",
									},
								},
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					creds, exists := jumperConfig.BasicAuth[feature.DefaultProviderKey]
					Expect(exists).To(BeTrue())
					Expect(creds.Username).To(Equal("route-user"))
					Expect(creds.Password).To(Equal("route-pass"))
				})
			})

			Context("when consumer has basic auth configured", func() {
				It("populates consumer key in BasicAuth map", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-basic",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "consumer-user",
											Password: "consumer-pass",
										},
									},
								},
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					creds, exists := jumperConfig.BasicAuth[plugin.ConsumerId("consumer-basic")]
					Expect(exists).To(BeTrue())
					Expect(creds.Username).To(Equal("consumer-user"))
					Expect(creds.Password).To(Equal("consumer-pass"))
				})
			})

			Context("when failover security overrides primary security", func() {
				It("uses failover credentials", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "failover-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type: gatewayv1.RouteTypeSecondary,
							Traffic: gatewayv1.Traffic{
								Failover: &gatewayv1.Failover{
									TargetZoneName: "zone-b",
									Security: gatewayv1.Security{
										M2M: &gatewayv1.Machine2MachineAuthentication{
											Basic: &gatewayv1.BasicAuthCredentials{
												Username: "failover-user",
												Password: "failover-pass",
											},
										},
									},
								},
							},
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Basic: &gatewayv1.BasicAuthCredentials{
										Username: "primary-user",
										Password: "primary-pass",
									},
								},
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Failover credentials should be used, not primary
					creds, exists := jumperConfig.BasicAuth[feature.DefaultProviderKey]
					Expect(exists).To(BeTrue())
					Expect(creds.Username).To(Equal("failover-user"))
					Expect(creds.Password).To(Equal("failover-pass"))
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

			Context("when secret manager fails for route password", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, _ string) (string, error) {
						return "", fmt.Errorf("secret manager unavailable")
					}

					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{},
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									Basic: &gatewayv1.BasicAuthCredentials{
										Username: "user",
										Password: "$<secret-ref>",
									},
								},
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get basic auth password for route test-route"))
					Expect(err.Error()).To(ContainSubstring("secret manager unavailable"))
				})
			})

			Context("when secret manager fails for consumer password", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, ref string) (string, error) {
						if ref == "$<consumer-secret-ref>" {
							return "", fmt.Errorf("consumer password fetch failed")
						}
						return ref, nil
					}

					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "failing-consumer",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "consumer-user",
											Password: "$<consumer-secret-ref>",
										},
									},
								},
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get basic auth password for consumer failing-consumer"))
					Expect(err.Error()).To(ContainSubstring("consumer password fetch failed"))
				})
			})
		})

		Context("edge cases", func() {
			Context("when consumer has nil security", func() {
				It("skips the consumer without error", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Type:    gatewayv1.RouteTypePrimary,
							Traffic: gatewayv1.Traffic{},
							Security: gatewayv1.Security{
								RealmName: "test-realm",
							},
						},
					}
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "no-security-consumer",
								// Security is nil
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// No entries should be added for the consumer
					Expect(jumperConfig.BasicAuth).To(BeEmpty())
				})
			})
		})
	})
})
