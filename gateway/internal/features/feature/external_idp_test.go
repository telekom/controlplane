// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExternalIDPFeature", func() {
	var (
		ctx     context.Context
		f       *feature.ExternalIDPFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceExternalIDPFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeExternalIDP", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeExternalIDP))
		})
	})

	Describe("Priority()", func() {
		It("returns 9", func() {
			Expect(f.Priority()).To(Equal(9))
		})
	})

	Describe("IsUsed()", func() {
		Context("when primary route has M2M ExternalIDP configured", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: false,
						Traffic:     gatewayv1.Traffic{},
						Security: gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								ExternalIDP: &gatewayv1.ExternalIdentityProvider{
									TokenEndpoint: "https://idp.example.com/token",
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has failover security with ExternalIDP", func() {
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
										ExternalIDP: &gatewayv1.ExternalIdentityProvider{
											TokenEndpoint: "https://idp.example.com/token",
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

		Context("when route is passthrough", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: true,
						Traffic:     gatewayv1.Traffic{},
						Security: gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								ExternalIDP: &gatewayv1.ExternalIdentityProvider{
									TokenEndpoint: "https://idp.example.com/token",
								},
							},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no ExternalIDP is configured", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:        gatewayv1.RouteTypePrimary,
						PassThrough: false,
						Traffic:     gatewayv1.Traffic{},
						Security: gatewayv1.Security{
							RealmName: "test-realm",
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
			Context("when provider uses OAuth2 client credentials", func() {
				It("adds token_endpoint header and populates JumperConfig OAuth for default provider", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "my-client-secret",
										},
									},
									Scopes: []string{"scope1", "scope2"},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Verify token_endpoint header is appended
					Expect(rtpPlugin.Config.Append.Headers).ToNot(BeNil())
					Expect(rtpPlugin.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://idp.example.com/token"))

					// Verify JumperConfig OAuth default entry
					oauth, exists := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(exists).To(BeTrue())
					Expect(oauth.ClientId).To(Equal("my-client-id"))
					Expect(oauth.ClientSecret).To(Equal("my-client-secret"))
					Expect(oauth.Scopes).To(Equal("scope1 scope2"))
					Expect(oauth.TokenRequest).To(Equal("header"))
					Expect(oauth.GrantType).To(Equal("client_credentials"))
				})
			})

			Context("when provider uses OAuth2 client credentials with client_secret_post", func() {
				It("sets TokenRequest to body in JumperConfig", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretPost,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "my-client-secret",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					oauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(oauth.TokenRequest).To(Equal("body"))
				})
			})

			Context("when provider uses OAuth2 with clientKey", func() {
				It("sets ClientKey in JumperConfig OAuth entry", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:  "my-client-id",
											ClientKey: "my-private-key",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					oauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(oauth.ClientId).To(Equal("my-client-id"))
					Expect(oauth.ClientKey).To(Equal("my-private-key"))
					Expect(oauth.ClientSecret).To(BeEmpty())
				})
			})

			Context("when provider uses OAuth2 with refreshToken", func() {
				It("sets RefreshToken in JumperConfig OAuth entry", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "refresh_token",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "my-client-secret",
											RefreshToken: "my-refresh-token",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					oauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(oauth.ClientId).To(Equal("my-client-id"))
					Expect(oauth.ClientSecret).To(Equal("my-client-secret"))
					Expect(oauth.RefreshToken).To(Equal("my-refresh-token"))
					Expect(oauth.GrantType).To(Equal("refresh_token"))
				})
			})

			Context("when provider uses basic auth", func() {
				It("populates JumperConfig OAuth with username and password", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "password",
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "admin",
											Password: "admin-pass",
										},
									},
									Scopes: []string{"read", "write"},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					oauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(oauth.Username).To(Equal("admin"))
					Expect(oauth.Password).To(Equal("admin-pass"))
					Expect(oauth.Scopes).To(Equal("read write"))
					Expect(oauth.GrantType).To(Equal("password"))
				})
			})

			Context("when consumer has OAuth2 client credentials", func() {
				It("adds per-consumer OAuth entry in JumperConfig", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "provider-id",
											ClientSecret: "provider-secret",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "consumer-a",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "consumer-client-id",
											ClientSecret: "consumer-client-secret",
										},
										Scopes: []string{"consumer-scope"},
									},
								},
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Verify provider entry
					providerOauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(providerOauth.ClientId).To(Equal("provider-id"))
					Expect(providerOauth.ClientSecret).To(Equal("provider-secret"))

					// Verify consumer entry
					consumerOauth, exists := jumperConfig.OAuth[plugin.ConsumerId("consumer-a")]
					Expect(exists).To(BeTrue())
					Expect(consumerOauth.ClientId).To(Equal("consumer-client-id"))
					Expect(consumerOauth.ClientSecret).To(Equal("consumer-client-secret"))
					Expect(consumerOauth.Scopes).To(Equal("consumer-scope"))
					Expect(consumerOauth.TokenRequest).To(Equal("header"))
					Expect(consumerOauth.GrantType).To(Equal("client_credentials"))
				})
			})

			Context("when consumer has basic auth credentials", func() {
				It("adds per-consumer OAuth entry with username and password", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretPost,
										GrantType:     "password",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "provider-id",
											ClientSecret: "provider-secret",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
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
										Scopes: []string{"basic-scope"},
									},
								},
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					consumerOauth, exists := jumperConfig.OAuth[plugin.ConsumerId("consumer-basic")]
					Expect(exists).To(BeTrue())
					Expect(consumerOauth.Username).To(Equal("consumer-user"))
					Expect(consumerOauth.Password).To(Equal("consumer-pass"))
					Expect(consumerOauth.Scopes).To(Equal("basic-scope"))
					Expect(consumerOauth.GrantType).To(Equal("password"))
				})
			})
		})

		Context("error handling", func() {
			Context("when no route in builder", func() {
				It("returns ErrNoRoute", func() {
					builder.EXPECT().GetRoute().Return(nil, false)
					builder.EXPECT().RequestTransformerPlugin().Return(&plugin.RequestTransformerPlugin{})

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoRoute))
				})
			})

			Context("when secret manager fails for provider client secret", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "$<secret-ref>",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get provider secret for route test-route"))
					Expect(err.Error()).To(ContainSubstring("secret manager unavailable"))
				})
			})

			Context("when secret manager fails for provider refresh token", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, ref string) (string, error) {
						if ref == "$<refresh-token-ref>" {
							return "", fmt.Errorf("refresh token fetch failed")
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
								M2M: &gatewayv1.Machine2MachineAuthentication{
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "refresh_token",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientKey:    "my-key",
											RefreshToken: "$<refresh-token-ref>",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get provider secret for route test-route"))
					Expect(err.Error()).To(ContainSubstring("refresh token fetch failed"))
				})
			})

			Context("when secret manager fails for provider basic auth password", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, _ string) (string, error) {
						return "", fmt.Errorf("password fetch failed")
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "password",
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "admin",
											Password: "$<password-ref>",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get provider secret for route test-route"))
					Expect(err.Error()).To(ContainSubstring("password fetch failed"))
				})
			})

			Context("when secret manager fails for consumer client secret", func() {
				It("returns a wrapped error", func() {
					originalGet := secretManagerApi.Get
					defer func() { secretManagerApi.Get = originalGet }()
					secretManagerApi.Get = func(_ context.Context, ref string) (string, error) {
						if ref == "$<consumer-secret-ref>" {
							return "", fmt.Errorf("consumer secret unavailable")
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
								M2M: &gatewayv1.Machine2MachineAuthentication{
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "failing-consumer",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "consumer-id",
											ClientSecret: "$<consumer-secret-ref>",
										},
									},
								},
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot get consumer secret for consumer failing-consumer"))
					Expect(err.Error()).To(ContainSubstring("consumer secret unavailable"))
				})
			})

			Context("when tokenRequest has unsupported value", func() {
				It("returns an error", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestMethod("unsupported_method"),
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "my-secret",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)

					err := f.Apply(ctx, builder)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unsupported tokenRequest value"))
					Expect(err.Error()).To(ContainSubstring("unsupported_method"))
				})
			})
		})

		Context("edge cases", func() {
			Context("when failover security overrides primary security", func() {
				It("uses failover settings for token_endpoint and credentials", func() {
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
											ExternalIDP: &gatewayv1.ExternalIdentityProvider{
												TokenEndpoint: "https://failover-idp.example.com/token",
												TokenRequest:  gatewayv1.TokenRequestClientSecretPost,
												GrantType:     "client_credentials",
												Client: &gatewayv1.OAuth2ClientCredentials{
													ClientId:     "failover-client-id",
													ClientSecret: "failover-client-secret",
												},
											},
											Scopes: []string{"failover-scope"},
										},
									},
								},
							},
							Security: gatewayv1.Security{
								M2M: &gatewayv1.Machine2MachineAuthentication{
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://primary-idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "primary-client-id",
											ClientSecret: "primary-client-secret",
										},
									},
									Scopes: []string{"primary-scope"},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Verify failover token_endpoint is used (not primary)
					Expect(rtpPlugin.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://failover-idp.example.com/token"))

					// Verify failover credentials are used
					oauth := jumperConfig.OAuth[feature.DefaultProviderKey]
					Expect(oauth.ClientId).To(Equal("failover-client-id"))
					Expect(oauth.ClientSecret).To(Equal("failover-client-secret"))
					Expect(oauth.Scopes).To(Equal("failover-scope"))
					Expect(oauth.TokenRequest).To(Equal("body"))
					Expect(oauth.GrantType).To(Equal("client_credentials"))
				})
			})

			Context("when consumers have no M2M credentials", func() {
				It("does not add consumer entries to JumperConfig", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "provider-id",
											ClientSecret: "provider-secret",
										},
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "no-m2m-consumer",
								// No security configured
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// Only the provider entry should exist
					Expect(jumperConfig.OAuth).To(HaveLen(1))
					_, exists := jumperConfig.OAuth[plugin.ConsumerId("no-m2m-consumer")]
					Expect(exists).To(BeFalse())
				})
			})

			Context("when provider has no credentials (neither client nor basic)", func() {
				It("only adds token_endpoint header without populating OAuth entry", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
										// No Client or Basic credentials
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// token_endpoint header is still added
					Expect(rtpPlugin.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://idp.example.com/token"))

					// No OAuth entry for provider since neither client nor basic is set
					Expect(jumperConfig.OAuth).To(BeEmpty())
				})
			})

			Context("when multiple consumers with mixed credentials exist", func() {
				It("adds OAuth entries for each consumer with credentials", func() {
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
									ExternalIDP: &gatewayv1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
										GrantType:     "client_credentials",
									},
								},
							},
						},
					}
					rtpPlugin := plugin.RequestTransformerPluginFromRoute(route)
					jumperConfig := plugin.NewJumperConfig()

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "oauth-consumer",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Client: &gatewayv1.OAuth2ClientCredentials{
											ClientId:     "oauth-id",
											ClientSecret: "oauth-secret",
										},
										Scopes: []string{"scope-a"},
									},
								},
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "basic-consumer",
								Security: &gatewayv1.ConsumeRouteSecurity{
									M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "basic-user",
											Password: "basic-pass",
										},
										Scopes: []string{"scope-b"},
									},
								},
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								ConsumerName: "no-creds-consumer",
								// No security
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().RequestTransformerPlugin().Return(rtpPlugin)
					builder.EXPECT().JumperConfig().Return(jumperConfig)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					// OAuth consumer
					oauthEntry, exists := jumperConfig.OAuth[plugin.ConsumerId("oauth-consumer")]
					Expect(exists).To(BeTrue())
					Expect(oauthEntry.ClientId).To(Equal("oauth-id"))
					Expect(oauthEntry.ClientSecret).To(Equal("oauth-secret"))
					Expect(oauthEntry.Scopes).To(Equal("scope-a"))

					// Basic consumer
					basicEntry, exists := jumperConfig.OAuth[plugin.ConsumerId("basic-consumer")]
					Expect(exists).To(BeTrue())
					Expect(basicEntry.Username).To(Equal("basic-user"))
					Expect(basicEntry.Password).To(Equal("basic-pass"))
					Expect(basicEntry.Scopes).To(Equal("scope-b"))

					// No-creds consumer should not exist
					_, exists = jumperConfig.OAuth[plugin.ConsumerId("no-creds-consumer")]
					Expect(exists).To(BeFalse())
				})
			})
		})
	})
})
