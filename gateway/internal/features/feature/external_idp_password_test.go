// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
	kongmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	secretManagerApi "github.com/telekom/controlplane/secret-manager/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func passwordIDP() *gatewayv1.ExternalIdentityProvider {
	return &gatewayv1.ExternalIdentityProvider{
		TokenEndpoint: "https://idp.example.com/token",
		TokenRequest:  gatewayv1.TokenRequestClientSecretBasic,
		GrantType:     gatewayv1.GrantTypePassword,
		Basic:         &gatewayv1.BasicAuthCredentials{Username: "provider-user", Password: "provider-pass"},
	}
}

func passwordConsumer() *gatewayv1.ConsumeRoute {
	return &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
		ConsumerName: "password-consumer",
		Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
			Basic:  &gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
			Scopes: []string{"consumer:read", "consumer:write"},
		}},
	}}
}

func primaryPasswordRoute() *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{Name: "password-route", Namespace: "test-ns"},
		Spec: gatewayv1.RouteSpec{
			Type: gatewayv1.RouteTypePrimary,
			Security: gatewayv1.Security{M2M: &gatewayv1.Machine2MachineAuthentication{
				ExternalIDP: passwordIDP(),
				Scopes:      []string{"provider:read"},
			}},
		},
	}
}

func secondaryPasswordRoute() *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{Name: "password-route", Namespace: "failover-ns"},
		Spec: gatewayv1.RouteSpec{
			Type: gatewayv1.RouteTypeSecondary,
			Traffic: gatewayv1.Traffic{Failover: &gatewayv1.Failover{
				TargetZoneName: "provider-zone",
				Security: gatewayv1.Security{M2M: &gatewayv1.Machine2MachineAuthentication{
					ExternalIDP: passwordIDP(),
					Scopes:      []string{"provider:read"},
				}},
			}},
		},
	}
}

var _ = Describe("External IDP password grant with consumer username/password and scopes", func() {
	var (
		ctx     context.Context
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	It("publishes password OAuth without backend Basic after an authentication change", func() {
		ctx = contextutil.WithEnv(ctx, "test-env")
		route := primaryPasswordRoute()
		route.Spec.Paths = []string{"/password/v1"}
		route.Spec.Backend.Upstreams = []gatewayv1.Upstream{
			{Scheme: "https", Hostname: "backend.example.com", Port: 443},
		}
		route.Spec.Security.TrustedIssuers = []string{"https://issuer.example.com/realms/test"}
		passwordSecurity := route.Spec.Security
		route.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{
			Basic: &gatewayv1.BasicAuthCredentials{Username: "old-provider", Password: "old-password"},
		}
		consumer := passwordConsumer()
		consumer.Spec.Route = *types.ObjectRefFromObject(route)
		consumer.Spec.Security.M2M.Scopes = nil

		var transformer *plugin.RequestTransformerPlugin
		var acl *plugin.AclPlugin
		kong := kongmock.NewMockKongClient(GinkgoT())
		kong.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
		kong.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).
			Run(func(_ context.Context, configured kongclient.CustomPlugin) {
				switch configured := configured.(type) {
				case *plugin.RequestTransformerPlugin:
					transformer = configured
				case *plugin.AclPlugin:
					acl = configured
				}
			}).Return(nil, nil)
		kong.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		publish := func(consumers ...*gatewayv1.ConsumeRoute) *plugin.JumperConfig {
			composed := features.NewFeatureBuilder(kong, route, nil, &gatewayv1.Gateway{})
			composed.EnableFeature(feature.InstanceAccessControlFeature)
			composed.EnableFeature(feature.InstanceExternalIDPFeature)
			composed.EnableFeature(feature.InstanceBasicAuthFeature)
			composed.EnableFeature(feature.InstanceCustomScopesFeature)
			composed.EnableFeature(feature.InstanceLastMileSecurityFeature)
			composed.AddAllowedConsumers(consumers...)
			Expect(composed.Build(ctx)).To(Succeed())
			Expect(transformer).NotTo(BeNil())
			configuration, err := plugin.FromBase64[plugin.JumperConfig](transformer.Config.Append.Headers.Get(plugin.JumperConfigKey))
			Expect(err).NotTo(HaveOccurred())
			return configuration
		}

		By("publishing the original backend Basic configuration")
		original := publish(consumer)
		Expect(original.BasicAuth).To(HaveKeyWithValue(plugin.ConsumerId("password-consumer"), plugin.BasicAuthCredentials{
			Username: "consumer-user", Password: "consumer-pass",
		}))

		By("replacing it with independent password identities and provider fallback")
		route.Spec.Security = passwordSecurity
		consumer.Spec.Security.M2M.Scopes = []string{"consumer:read", "consumer:write"}
		scopesOnly := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
			Route: *types.ObjectRefFromObject(route), ConsumerName: "scopes-consumer",
			Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
				Scopes: []string{"consumer:read"},
			}},
		}}
		fallback := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
			Route: *types.ObjectRefFromObject(route), ConsumerName: "fallback-consumer",
		}}
		updated := publish(consumer, scopesOnly, fallback)
		Expect(updated.BasicAuth).To(BeEmpty())
		Expect(updated.OAuth).To(Equal(map[plugin.ConsumerId]plugin.OauthCredentials{
			"default": {
				Username: "provider-user", Password: "provider-pass", GrantType: "password", Scopes: "provider:read",
			},
			"password-consumer": {
				Username: "consumer-user", Password: "consumer-pass", GrantType: "password", Scopes: "consumer:read consumer:write",
			},
			"scopes-consumer": {Scopes: "consumer:read", GrantType: "password"},
		}))
		Expect(transformer.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://idp.example.com/token"))
		Expect(acl).NotTo(BeNil())
		Expect(acl.Config.Allow.Values()).To(ConsistOf("password-consumer", "scopes-consumer", "fallback-consumer"))

		By("preserving a Basic-only consumer alongside the newly supported combination")
		legacy := passwordConsumer()
		legacy.Spec.ConsumerName = "basic-only-consumer"
		legacy.Spec.Route = *types.ObjectRefFromObject(route)
		legacy.Spec.Security.M2M.Scopes = nil
		mixed := publish(consumer, legacy)
		Expect(mixed.BasicAuth).To(Equal(map[plugin.ConsumerId]plugin.BasicAuthCredentials{
			"basic-only-consumer": {Username: "consumer-user", Password: "consumer-pass"},
		}))
		Expect(mixed.OAuth[plugin.ConsumerId("password-consumer")]).To(Equal(updated.OAuth[plugin.ConsumerId("password-consumer")]))
		Expect(mixed.OAuth).NotTo(HaveKey(plugin.ConsumerId("basic-only-consumer")))
	})

	DescribeTable("publishes exactly one authentication method per consumer",
		func(providerGrant gatewayv1.GrantType, configure func(*gatewayv1.ConsumerMachine2MachineAuthentication),
			expectedBasic map[plugin.ConsumerId]plugin.BasicAuthCredentials, expectedConsumerOAuth *plugin.OauthCredentials,
		) {
			originalGet := secretManagerApi.Get
			defer func() { secretManagerApi.Get = originalGet }()
			secretManagerApi.Get = func(_ context.Context, ref string) (string, error) { return ref, nil }

			ctx = contextutil.WithEnv(ctx, "test-env")
			route := primaryPasswordRoute()
			route.Spec.Paths = []string{"/password/v1"}
			route.Spec.Backend.Upstreams = []gatewayv1.Upstream{{Scheme: "https", Hostname: "backend.example.com", Port: 443}}
			route.Spec.Security.TrustedIssuers = []string{"https://issuer.example.com/realms/test"}
			route.Spec.Security.M2M.ExternalIDP.GrantType = providerGrant
			// Username/password require the password grant, so other grant types use provider client credentials.
			if providerGrant != gatewayv1.GrantTypePassword {
				route.Spec.Security.M2M.ExternalIDP.Basic = nil
				route.Spec.Security.M2M.ExternalIDP.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "provider-client", ClientSecret: "provider-secret"}
				if providerGrant == gatewayv1.GrantTypeRefreshToken {
					route.Spec.Security.M2M.ExternalIDP.Client.RefreshToken = "provider-refresh-token"
				}
			}
			consumer := passwordConsumer()
			consumer.Spec.Route = *types.ObjectRefFromObject(route)
			consumer.Spec.Security.M2M.Scopes = nil
			configure(consumer.Spec.Security.M2M)

			var transformer *plugin.RequestTransformerPlugin
			kong := kongmock.NewMockKongClient(GinkgoT())
			kong.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			kong.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).
				Run(func(_ context.Context, configured kongclient.CustomPlugin) {
					if configured, ok := configured.(*plugin.RequestTransformerPlugin); ok {
						transformer = configured
					}
				}).Return(nil, nil)
			kong.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			composed := features.NewFeatureBuilder(kong, route, nil, &gatewayv1.Gateway{})
			composed.EnableFeature(feature.InstanceAccessControlFeature)
			composed.EnableFeature(feature.InstanceExternalIDPFeature)
			composed.EnableFeature(feature.InstanceBasicAuthFeature)
			composed.EnableFeature(feature.InstanceCustomScopesFeature)
			composed.EnableFeature(feature.InstanceLastMileSecurityFeature)
			composed.AddAllowedConsumers(consumer)
			Expect(composed.Build(ctx)).To(Succeed())
			Expect(transformer).NotTo(BeNil())
			jumperConfig, err := plugin.FromBase64[plugin.JumperConfig](transformer.Config.Append.Headers.Get(plugin.JumperConfigKey))
			Expect(err).NotTo(HaveOccurred())

			if len(expectedBasic) == 0 {
				Expect(jumperConfig.BasicAuth).To(BeEmpty())
			} else {
				Expect(jumperConfig.BasicAuth).To(Equal(expectedBasic))
			}
			if expectedConsumerOAuth == nil {
				Expect(jumperConfig.OAuth).NotTo(HaveKey(plugin.ConsumerId("password-consumer")))
			} else {
				Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("password-consumer"), *expectedConsumerOAuth))
			}
			Expect(jumperConfig.OAuth).To(HaveKey(feature.DefaultProviderKey))
		},
		Entry("Basic only stays backend Basic", gatewayv1.GrantTypePassword,
			func(*gatewayv1.ConsumerMachine2MachineAuthentication) {},
			map[plugin.ConsumerId]plugin.BasicAuthCredentials{"password-consumer": {Username: "consumer-user", Password: "consumer-pass"}},
			nil),
		Entry("scopes with an inherited password grant use the password grant", gatewayv1.GrantTypePassword,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) { m2m.Scopes = []string{"consumer:read"} },
			nil,
			&plugin.OauthCredentials{Username: "consumer-user", Password: "consumer-pass", Scopes: "consumer:read", GrantType: "password"}),
		Entry("scopes with an inherited client_credentials grant use the provider credentials with consumer scopes", gatewayv1.GrantTypeClientCredentials,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) { m2m.Scopes = []string{"consumer:read"} },
			nil,
			&plugin.OauthCredentials{Scopes: "consumer:read", GrantType: "client_credentials"}),
		Entry("scopes with an explicit client_credentials grant use the provider credentials with consumer scopes", gatewayv1.GrantTypePassword,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Scopes = []string{"consumer:read"}
				m2m.GrantType = gatewayv1.GrantTypeClientCredentials
			},
			nil,
			&plugin.OauthCredentials{Scopes: "consumer:read", GrantType: "password"}),
		Entry("client credentials with a client_credentials grant drop username/password", gatewayv1.GrantTypeClientCredentials,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
				m2m.Scopes = []string{"consumer:read"}
			},
			nil,
			&plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret", Scopes: "consumer:read",
				TokenRequest: "header", GrantType: "client_credentials",
			}),
		Entry("an explicit password grant without scopes uses the password grant", gatewayv1.GrantTypeClientCredentials,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.GrantType = gatewayv1.GrantTypePassword
			},
			nil,
			&plugin.OauthCredentials{Username: "consumer-user", Password: "consumer-pass", GrantType: "password"}),
		Entry("client credentials behind an inherited password grant use the provider credentials with consumer scopes", gatewayv1.GrantTypePassword,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Basic = nil
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
				m2m.Scopes = []string{"consumer:read"}
			},
			nil,
			&plugin.OauthCredentials{Scopes: "consumer:read", GrantType: "password"}),
		Entry("client credentials without a refresh token behind an inherited refresh_token grant use the provider credentials", gatewayv1.GrantTypeRefreshToken,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Basic = nil
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
				m2m.Scopes = []string{"consumer:read"}
			},
			nil,
			&plugin.OauthCredentials{Scopes: "consumer:read", GrantType: "refresh_token"}),
		Entry("client credentials with a refresh token use the inherited refresh_token grant", gatewayv1.GrantTypeRefreshToken,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Basic = nil
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{
					ClientId: "consumer-client", ClientSecret: "consumer-secret", RefreshToken: "consumer-refresh-token",
				}
			},
			nil,
			&plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret", RefreshToken: "consumer-refresh-token",
				TokenRequest: "header", GrantType: "refresh_token",
			}),
		Entry("client and username/password with a password grant use both", gatewayv1.GrantTypePassword,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
			},
			nil,
			&plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret", Username: "consumer-user", Password: "consumer-pass",
				TokenRequest: "header", GrantType: "password",
			}),
		Entry("client and username/password with an explicit password grant override an inherited client_credentials grant", gatewayv1.GrantTypeClientCredentials,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
				m2m.Scopes = []string{"consumer:read"}
				m2m.GrantType = gatewayv1.GrantTypePassword
			},
			nil,
			&plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret", Username: "consumer-user", Password: "consumer-pass",
				Scopes: "consumer:read", TokenRequest: "header", GrantType: "password",
			}),
		Entry("client credentials with a refresh token and an explicit refresh_token grant override an inherited client_credentials grant", gatewayv1.GrantTypeClientCredentials,
			func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Basic = nil
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{
					ClientId: "consumer-client", ClientSecret: "consumer-secret", RefreshToken: "consumer-refresh-token",
				}
				m2m.Scopes = []string{"consumer:read"}
				m2m.GrantType = gatewayv1.GrantTypeRefreshToken
			},
			nil,
			&plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret", RefreshToken: "consumer-refresh-token",
				Scopes: "consumer:read", TokenRequest: "header", GrantType: "refresh_token",
			}),
	)

	Describe("BasicAuthFeature", func() {
		DescribeTable("ignores consumers without Basic credentials", func(security *gatewayv1.ConsumeRouteSecurity) {
			consumer := passwordConsumer()
			consumer.Spec.Security = security
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(primaryPasswordRoute(), true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
			builder.EXPECT().JumperConfig().Return(jumperConfig)

			Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeFalse())
			Expect(feature.InstanceBasicAuthFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.BasicAuth).To(BeEmpty())
		},
			Entry("without security", nil),
			Entry("without M2M", &gatewayv1.ConsumeRouteSecurity{}),
			Entry("without Basic", &gatewayv1.ConsumeRouteSecurity{
				M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{Scopes: []string{"consumer:read"}},
			}),
		)

		DescribeTable("preserves backend Basic for existing credential shapes", func(grantType gatewayv1.GrantType) {
			route := primaryPasswordRoute()
			route.Spec.Security.M2M.ExternalIDP.GrantType = grantType
			consumer := passwordConsumer()
			consumer.Spec.Security.M2M.Scopes = nil
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
			builder.EXPECT().JumperConfig().Return(jumperConfig)

			Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeTrue())
			Expect(feature.InstanceBasicAuthFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.BasicAuth).To(HaveKeyWithValue(plugin.ConsumerId(consumer.Spec.ConsumerName), plugin.BasicAuthCredentials{
				Username: "consumer-user", Password: "consumer-pass",
			}))
		},
			Entry("password without scopes", gatewayv1.GrantTypePassword),
			Entry("client credentials", gatewayv1.GrantTypeClientCredentials),
			Entry("authorization code", gatewayv1.GrantTypeAuthorizationCode),
		)

		It("does not publish dual password-grant credentials as backend Basic without scopes", func() {
			route := primaryPasswordRoute()
			consumer := passwordConsumer()
			consumer.Spec.Security.M2M.Scopes = nil
			consumer.Spec.Security.M2M.Client = &gatewayv1.OAuth2ClientCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret",
			}
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
			builder.EXPECT().JumperConfig().Return(jumperConfig)

			Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeFalse())
			Expect(feature.InstanceBasicAuthFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.BasicAuth).To(BeEmpty())
		})

		DescribeTable("does not publish consumer credentials as backend Basic auth", func(route *gatewayv1.Route) {
			builder.EXPECT().GetRoute().Return(route, true).Maybe()
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()}).Maybe()

			Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeFalse())
		},
			Entry("on a primary route", primaryPasswordRoute()),
			Entry("on a failover-secondary route", secondaryPasswordRoute()),
		)

		DescribeTable("never treats credentials with scopes, client credentials or a grant type as backend Basic",
			func(configure func(*gatewayv1.ConsumerMachine2MachineAuthentication)) {
				route := primaryPasswordRoute()
				route.Spec.Security.M2M.ExternalIDP.GrantType = gatewayv1.GrantTypeClientCredentials
				consumer := passwordConsumer()
				consumer.Spec.Security.M2M.Scopes = nil
				configure(consumer.Spec.Security.M2M)
				jumperConfig := plugin.NewJumperConfig()
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})
				builder.EXPECT().JumperConfig().Return(jumperConfig)

				Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeFalse())
				Expect(feature.InstanceBasicAuthFeature.Apply(ctx, builder)).To(Succeed())
				Expect(jumperConfig.BasicAuth).To(BeEmpty())
			},
			Entry("scopes with an inherited client_credentials grant", func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Scopes = []string{"consumer:read"}
			}),
			Entry("scopes with an explicit client_credentials grant", func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Scopes = []string{"consumer:read"}
				m2m.GrantType = gatewayv1.GrantTypeClientCredentials
			}),
			Entry("client credentials", func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "consumer-client", ClientSecret: "consumer-secret"}
			}),
			Entry("only a password grant", func(m2m *gatewayv1.ConsumerMachine2MachineAuthentication) {
				m2m.GrantType = gatewayv1.GrantTypePassword
			}),
		)

		It("adds no BasicAuth entry even when applied to a primary route", func() {
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(primaryPasswordRoute(), true)
			builder.EXPECT().JumperConfig().Return(jumperConfig).Maybe()
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()}).Maybe()

			Expect(feature.InstanceBasicAuthFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.BasicAuth).To(BeEmpty())
		})
	})

	Describe("ExternalIDPFeature", func() {
		It("requests scopes-only consumers with their own scopes and the provider credentials", func() {
			route := primaryPasswordRoute()
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			scopesOnly := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
				ConsumerName: "scopes-consumer",
				Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Scopes: []string{"consumer:read"},
				}},
			}}
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{scopesOnly})

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("scopes-consumer"), plugin.OauthCredentials{
				Scopes: "consumer:read", GrantType: "password",
			}))
			Expect(jumperConfig.OAuth[feature.DefaultProviderKey].Scopes).To(Equal("provider:read"))
		})

		It("adds no scopes-only consumer entry without provider credentials to complete it", func() {
			route := primaryPasswordRoute()
			route.Spec.Security.M2M.ExternalIDP.Basic = nil
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			scopesOnly := &gatewayv1.ConsumeRoute{Spec: gatewayv1.ConsumeRouteSpec{
				ConsumerName: "scopes-consumer",
				Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Scopes: []string{"consumer:read"},
				}},
			}}
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{scopesOnly})

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth).To(BeEmpty())
		})

		It("does not publish provider credentials that cannot satisfy the grant type", func() {
			route := primaryPasswordRoute()
			route.Spec.Security.M2M.ExternalIDP.Basic = nil
			route.Spec.Security.M2M.ExternalIDP.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "provider", ClientSecret: "provider-secret"}
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()})

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth).NotTo(HaveKey(feature.DefaultProviderKey))
			Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId("password-consumer")))
		})

		It("publishes provider client and username/password credentials together for the password grant", func() {
			originalGet := secretManagerApi.Get
			defer func() { secretManagerApi.Get = originalGet }()
			secretManagerApi.Get = func(_ context.Context, ref string) (string, error) { return ref, nil }

			route := primaryPasswordRoute()
			route.Spec.Security.M2M.ExternalIDP.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "provider", ClientSecret: "provider-secret"}
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetAllowedConsumers().Return(nil)

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(feature.DefaultProviderKey, plugin.OauthCredentials{
				ClientId: "provider", ClientSecret: "provider-secret", Username: "provider-user", Password: "provider-pass",
				TokenRequest: "header", Scopes: "provider:read", GrantType: "password",
			}))
		})

		DescribeTable("publishes separate provider and consumer password-grant entries",
			func(route *gatewayv1.Route) {
				jumperConfig := plugin.NewJumperConfig()
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
				builder.EXPECT().JumperConfig().Return(jumperConfig)
				builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()})

				Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())

				Expect(jumperConfig.OAuth[feature.DefaultProviderKey]).To(Equal(plugin.OauthCredentials{
					Username: "provider-user", Password: "provider-pass", Scopes: "provider:read",
					GrantType: "password",
				}))
				Expect(jumperConfig.OAuth[plugin.ConsumerId("password-consumer")]).To(Equal(plugin.OauthCredentials{
					Username: "consumer-user", Password: "consumer-pass", Scopes: "consumer:read consumer:write",
					GrantType: "password",
				}))
			},
			Entry("primary route", primaryPasswordRoute()),
			Entry("failover-secondary route", secondaryPasswordRoute()),
		)

		It("forwards client credentials and username/password together to Jumper", func() {
			originalGet := secretManagerApi.Get
			defer func() { secretManagerApi.Get = originalGet }()
			secretManagerApi.Get = func(_ context.Context, ref string) (string, error) {
				return "resolved-" + ref, nil
			}

			consumer := passwordConsumer()
			consumer.Spec.Security.M2M.Client = &gatewayv1.OAuth2ClientCredentials{
				ClientId: "consumer-client", ClientSecret: "consumer-secret",
			}
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(primaryPasswordRoute(), true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(primaryPasswordRoute()))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth[plugin.ConsumerId(consumer.Spec.ConsumerName)]).To(Equal(plugin.OauthCredentials{
				ClientId: "consumer-client", ClientSecret: "resolved-consumer-secret",
				Username: "consumer-user", Password: "resolved-consumer-pass",
				Scopes: "consumer:read consumer:write", GrantType: "password", TokenRequest: "header",
			}))
		})
	})
})
