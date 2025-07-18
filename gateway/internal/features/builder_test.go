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
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

var _ = Describe("FeatureBuilder", Ordered, func() {
	Context("Registering", Ordered, func() {
		It("should be registered", func() {
			kc := mock.NewMockKongClient(mockCtrl)

			builder := features.NewFeatureBuilder(kc, route, realm, gateway)

			builder.EnableFeature(feature.InstancePassThroughFeature)
			builder.EnableFeature(feature.InstanceAccessControlFeature)
			builder.EnableFeature(feature.InstanceLastMileSecurityFeature)
			builder.EnableFeature(feature.InstanceCustomScopesFeature)
			builder.EnableFeature(feature.InstanceLoadBalancingFeature)
			builder.EnableFeature(feature.InstanceExternalIDPFeature)

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())
			Expect(b.Features).To(HaveLen(6))
		})

	})

	Context("Applying and Creating", Ordered, func() {
		var ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, "test")
		BeforeEach(func() {
			mockKc = mock.NewMockKongClient(mockCtrl)
		})

		It("should fail if no upstream is set", func() {
			// No features enabled

			builder := features.NewFeatureBuilder(mockKc, route, realm, gateway)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("upstream is not set"))
		})

		It("should apply the PassThrough feature", func() {
			passThroughRoute := route.DeepCopy()
			passThroughRoute.Spec.PassThrough = true
			builder := features.NewFeatureBuilder(mockKc, passThroughRoute, realm, gateway)
			builder.EnableFeature(feature.InstancePassThroughFeature)

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, passThroughRoute, gomock.Any()).Return(nil).Times(1)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the upstream is the real-upstream")
			Expect(b.Upstream).NotTo(BeNil())
			Expect(b.Upstream.GetScheme()).To(Equal("http"))
			Expect(b.Upstream.GetHost()).To(Equal("upstream.url"))
			Expect(b.Upstream.GetPort()).To(Equal(8080))
			Expect(b.Upstream.GetPath()).To(Equal("/api/v1"))
		})

		It("should apply the AccessControl feature", func() {
			acRoute := route.DeepCopy()
			acRoute.Spec.Downstreams[0].IssuerUrl = "https://issuer.url"
			acRoute.Spec.PassThrough = true
			builder := features.NewFeatureBuilder(mockKc, acRoute, realm, gateway)

			consumeRoute := NewMockConsumeRoute(*types.ObjectRefFromObject(acRoute))
			builder.AddAllowedConsumers(consumeRoute)

			builder.EnableFeature(feature.InstancePassThroughFeature)
			builder.EnableFeature(feature.InstanceAccessControlFeature)

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, acRoute, gomock.Any()).Return(nil).Times(1)
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(2)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the upstream is the real-upstream")
			Expect(b.Upstream).ToNot(BeNil())

			By("Checking that the plugins are set")
			Expect(b.Plugins).To(HaveLen(2))

			By("checking the jwt plugin")
			jwtPlugin, ok := b.Plugins["jwt"].(*plugin.JwtPlugin)
			Expect(ok).To(BeTrue())
			Expect(jwtPlugin.Config.AllowedIss.Contains("https://issuer.url")).To(BeTrue())

			By("checking the acl plugins")
			aclPlugin, ok := b.Plugins["acl"].(*plugin.AclPlugin)
			Expect(ok).To(BeTrue())
			Expect(aclPlugin.Config.Allow.Contains("gateway")).To(BeTrue())
			Expect(aclPlugin.Config.Allow.Contains("test")).To(BeTrue())
			Expect(aclPlugin.Config.Allow.Contains("test-consumer-name")).To(BeTrue())
		})

		It("should correctly apply the LastMileSecurity feature for a real-route", func() {
			lmsRoute := route.DeepCopy()
			lmsRoute.Spec.PassThrough = false

			builder := features.NewFeatureBuilder(mockKc, lmsRoute, realm, gateway)

			builder.EnableFeature(feature.InstanceLastMileSecurityFeature)

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, lmsRoute, gomock.Any()).Return(nil).Times(1)
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the upstream is the proxy-upstream")
			Expect(b.Upstream).ToNot(BeNil())
			Expect(b.Upstream).To(Equal(client.NewUpstreamOrDie("http://localhost:8080/proxy")))

			By("Checking that the plugins are set")
			Expect(b.Plugins).To(HaveLen(1))

			By("checking the request-transformer plugin")
			rtPlugin, ok := b.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
			Expect(ok).To(BeTrue())

			By("checking the request-transformer plugin config")
			Expect(rtPlugin.Config.Replace.Headers.Get("Authorization")).To(Equal("$(headers['consumer-token'] or headers['Authorization'])"))
			Expect(rtPlugin.Config.Remove.Headers.Contains("consumer-token")).To(BeTrue())

			Expect(rtPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("http://upstream.url:8080/api/v1"))
			Expect(rtPlugin.Config.Append.Headers.Get("api_base_path")).To(Equal("/api/v1"))
			Expect(b.JumperConfig()).To(Equal(plugin.NewJumperConfig())) // empty jumper config

			Expect(rtPlugin.Config.Add.Headers.Get("environment")).To(Equal("test"))
			Expect(rtPlugin.Config.Add.Headers.Get("realm")).To(Equal("test-realm"))
		})

		It("should correctly apply the LastMileSecurity feature for a proxy-route", func() {
			lmsRoute := route.DeepCopy()
			lmsRoute.Spec.PassThrough = false
			lmsRoute.Spec.Upstreams[0] = gatewayv1.Upstream{
				Scheme:       "http",
				Host:         "upstream.url",
				Port:         8080,
				Path:         "/api/v1",
				IssuerUrl:    "https://upstream.issuer.url",
				ClientId:     "gateway",
				ClientSecret: "topsecret",
			}

			builder := features.NewFeatureBuilder(mockKc, lmsRoute, realm, gateway)

			builder.EnableFeature(feature.InstanceLastMileSecurityFeature)

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, lmsRoute, gomock.Any()).Return(nil).Times(1)
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that the upstream is the proxy-upstream")
			Expect(b.Upstream).ToNot(BeNil())
			Expect(b.Upstream).To(Equal(client.NewUpstreamOrDie("http://localhost:8080/proxy")))

			By("Checking that the plugins are set")
			Expect(b.Plugins).To(HaveLen(1))

			By("checking the request-transformer plugin")
			rtPlugin, ok := b.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
			Expect(ok).To(BeTrue())

			By("checking the request-transformer plugin config")
			Expect(rtPlugin.Config.Append.Headers.Get("remote_api_url")).To(Equal("http://upstream.url:8080/api/v1"))

			Expect(rtPlugin.Config.Append.Headers.Get("issuer")).To(Equal("https://upstream.issuer.url"))
			Expect(rtPlugin.Config.Append.Headers.Get("client_id")).To(Equal("gateway"))
			Expect(rtPlugin.Config.Append.Headers.Get("client_secret")).To(Equal("topsecret"))
		})

		It("should apply the CustomScopes feature", func() {
			scopesRoute := route.DeepCopy()
			consumeRoute := NewMockConsumeRoute(*types.ObjectRefFromObject(scopesRoute))
			builder := features.NewFeatureBuilder(mockKc, scopesRoute, realm, gateway)
			builder.AddAllowedConsumers(consumeRoute)
			builder.EnableFeature(feature.InstanceCustomScopesFeature)
			builder.EnableFeature(feature.InstanceLastMileSecurityFeature)

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, scopesRoute, gomock.Any()).Return(nil).Times(1)
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			By("building the features")
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())

			b, ok := builder.(*features.Builder)
			Expect(ok).To(BeTrue())

			By("Checking that jumperConfig is filled with the scopes")
			Expect(b.JumperConfig).ToNot(BeNil())
			Expect(b.JumperConfig().OAuth[plugin.ConsumerId("default")].Scopes).To(Equal("scope1"))
			Expect(b.JumperConfig().OAuth[plugin.ConsumerId(consumeRoute.Spec.ConsumerName)].Scopes).To(Equal("scope1 scope2"))
		})

		It("should correctly apply the RateLimit feature", func() {
			// TBD
			Expect(true).To(BeTrue())
		})

		// TBD other features

	})

})
