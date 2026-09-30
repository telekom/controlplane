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
	})

	Describe("BasicAuthFeature", func() {
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

		DescribeTable("does not publish consumer credentials as backend Basic auth", func(route *gatewayv1.Route) {
			builder.EXPECT().GetRoute().Return(route, true).Maybe()
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()}).Maybe()

			Expect(feature.InstanceBasicAuthFeature.IsUsed(ctx, builder)).To(BeFalse())
		},
			Entry("on a primary route", primaryPasswordRoute()),
			Entry("on a failover-secondary route", secondaryPasswordRoute()),
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
		It("preserves whole-default fallback for scopes-only consumers", func() {
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
			Expect(jumperConfig.OAuth).NotTo(HaveKey(plugin.ConsumerId("scopes-consumer")))
			Expect(jumperConfig.OAuth[feature.DefaultProviderKey].Scopes).To(Equal("provider:read"))
		})

		It("preserves an existing client-only provider default", func() {
			route := primaryPasswordRoute()
			route.Spec.Security.M2M.ExternalIDP.Basic = nil
			route.Spec.Security.M2M.ExternalIDP.Client = &gatewayv1.OAuth2ClientCredentials{ClientId: "provider", ClientSecret: "provider-secret"}
			jumperConfig := plugin.NewJumperConfig()
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTransformerPlugin().Return(plugin.RequestTransformerPluginFromRoute(route))
			builder.EXPECT().JumperConfig().Return(jumperConfig)
			builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{passwordConsumer()})

			Expect(feature.InstanceExternalIDPFeature.Apply(ctx, builder)).To(Succeed())
			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(feature.DefaultProviderKey, plugin.OauthCredentials{
				ClientId: "provider", ClientSecret: "provider-secret", GrantType: "password",
				TokenRequest: "header", Scopes: "provider:read",
			}))
			Expect(jumperConfig.OAuth).To(HaveKey(plugin.ConsumerId("password-consumer")))
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
	})
})
