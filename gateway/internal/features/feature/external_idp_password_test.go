// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
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

	Describe("BasicAuthFeature", func() {
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
