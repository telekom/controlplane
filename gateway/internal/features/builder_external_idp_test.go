// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0
package features_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

var _ = Describe("FeatureBuilder externalIDP", Ordered, func() {
	var ctx = context.Background()
	ctx = contextutil.WithEnv(ctx, "test")
	BeforeEach(func() {
		mockKc = mock.NewMockKongClient(mockCtrl)
	})

	Context("Applying and Creating", Ordered, func() {

		BeforeEach(func() {
			mockKc = mock.NewMockKongClient(mockCtrl)
		})

		It("should apply the ExternalIDPConfig with Oauth", func() {
			externalIDPRoute := externalIDPProviderRouteOAuth()
			configureExternalIDPMocks(ctx, externalIDPRoute)

			By("building the features")
			builder := features.NewFeatureBuilder(mockKc, externalIDPRoute, realm, gateway)
			builder.EnableFeature(feature.InstanceExternalIDPFeature)
			builder.SetUpstream(externalIDPRoute.Spec.Upstreams[0])

			By("defining the consumer oauth config")
			consumerRoute := NewMockConsumeRoute(*types.ObjectRefFromObject(externalIDPRoute))
			consumerRoute.Spec.Security.M2M.Scopes = []string{"team:application"}
			consumerRoute.Spec.Security.M2M.Client = &gatewayv1.OAuth2ClientCredentials{
				ClientId:     "test-user",
				ClientSecret: "******",
			}
			consumerRoute.Spec.Security.M2M.Scopes = []string{"idp:group"}
			builder.AddAllowedConsumers(consumerRoute)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the plugins are set")
			Expect(b.Plugins).To(HaveLen(1))

			By("checking the request-transformer plugin")
			rtPlugin, ok := b.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
			Expect(ok).To(BeTrue())

			By("checking the request-transformer plugin config")
			Expect(rtPlugin.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://example.com/tokenEndpoint"))

			By("checking the jumper plugin")
			jumperConfig := builder.JumperConfig()
			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("default"), plugin.OauthCredentials{
				Scopes:       "admin:application",
				ClientId:     "gateway",
				ClientSecret: "topsecret",
				GrantType:    "client_credentials",
				TokenRequest: "header",
			}))

			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("test-consumer-name"), plugin.OauthCredentials{
				Scopes:       "idp:group",
				ClientId:     "test-user",
				ClientSecret: "******",
				GrantType:    "client_credentials",
				TokenRequest: "header",
			}))

		})

		It("should apply the ExternalIDPConfig with Basic", func() {
			externalIDPRoute := externalIDPProviderRouteBasic()
			configureExternalIDPMocks(ctx, externalIDPRoute)

			By("building the features")
			builder := features.NewFeatureBuilder(mockKc, externalIDPRoute, realm, gateway)
			builder.EnableFeature(feature.InstanceExternalIDPFeature)
			builder.SetUpstream(externalIDPRoute.Spec.Upstreams[0])

			By("defining the consumer oauth config")
			consumerRoute := NewMockConsumeRoute(*types.ObjectRefFromObject(externalIDPRoute))
			consumerRoute.Spec.Security.M2M.Scopes = []string{"team:application"}
			consumerRoute.Spec.Security.M2M.Basic = &gatewayv1.BasicAuthCredentials{
				Username: "test-user",
				Password: "******",
			}
			consumerRoute.Spec.Security.M2M.Scopes = []string{"idp:group"}
			builder.AddAllowedConsumers(consumerRoute)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the plugins are set")
			Expect(b.Plugins).To(HaveLen(1))

			By("checking the request-transformer plugin")
			rtPlugin, ok := b.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
			Expect(ok).To(BeTrue())

			By("checking the request-transformer plugin config")
			Expect(rtPlugin.Config.Append.Headers.Get("token_endpoint")).To(Equal("https://example.com/tokenEndpoint"))

			By("checking the jumper plugin")
			jumperConfig := builder.JumperConfig()
			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("default"), plugin.OauthCredentials{
				Username:  "user",
				Password:  "*** ***",
				GrantType: "password",
				Scopes:    "admin:application",
			}))

			Expect(jumperConfig.OAuth).To(HaveKeyWithValue(plugin.ConsumerId("test-consumer-name"), plugin.OauthCredentials{
				Username:  "test-user",
				Password:  "******",
				GrantType: "password",
				Scopes:    "idp:group",
			}))

		})

	})

})

func externalIDPProviderRouteOAuth() *gatewayv1.Route {
	eIDPRoute := route.DeepCopy()
	eIDPRoute.Spec.PassThrough = false
	eIDPRoute.Spec.Upstreams[0] = gatewayv1.Upstream{
		Scheme: "http",
		Host:   "upstream.url",
		Port:   8080,
		Path:   "/api/v1",
	}

	eIDPRoute.Spec.Security = &gatewayv1.Security{
		M2M: &gatewayv1.Machine2MachineAuthentication{
			ExternalIDP: &gatewayv1.ExternalIdentityProvider{
				TokenEndpoint: "https://example.com/tokenEndpoint",
				TokenRequest:  "header",
				GrantType:     "client_credentials",
				Client: &gatewayv1.OAuth2ClientCredentials{
					ClientId:     "gateway",
					ClientSecret: "topsecret",
				},
			},
			Scopes: []string{"admin:application"},
		},
	}

	return eIDPRoute
}

func externalIDPProviderRouteBasic() *gatewayv1.Route {
	eIDPRoute := route.DeepCopy()
	eIDPRoute.Spec.PassThrough = false
	eIDPRoute.Spec.Upstreams[0] = gatewayv1.Upstream{
		Scheme: "http",
		Host:   "upstream.url",
		Port:   8080,
		Path:   "/api/v1",
	}

	eIDPRoute.Spec.Security = &gatewayv1.Security{
		M2M: &gatewayv1.Machine2MachineAuthentication{
			ExternalIDP: &gatewayv1.ExternalIdentityProvider{
				TokenEndpoint: "https://example.com/tokenEndpoint",
				TokenRequest:  "header",
				GrantType:     "password",
				Basic: &gatewayv1.BasicAuthCredentials{
					Username: "user",
					Password: "*** ***",
				},
			},
			Scopes: []string{"admin:application"},
		},
	}

	return eIDPRoute
}

func configureExternalIDPMocks(ctx context.Context, externalIDPRoute *gatewayv1.Route) {
	mockKc.EXPECT().CreateOrReplaceRoute(ctx, externalIDPRoute, gomock.Any()).Return(nil).Times(1)
	mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
	mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)
}
