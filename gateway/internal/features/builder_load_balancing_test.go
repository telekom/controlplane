// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

func NewLoadBalancingUpstreams(isProxyRoute bool) []gatewayv1.Upstream {
	var issuerUrl string
	if isProxyRoute {
		issuerUrl = "https://upstream.issuer.url"
	} else {
		issuerUrl = ""
	}

	return []gatewayv1.Upstream{
		{
			Weight:    2,
			Scheme:    "http",
			Host:      "upstream.url",
			Port:      8080,
			Path:      "/api/v1",
			IssuerUrl: issuerUrl,
		},
		{
			Weight:    1,
			Scheme:    "http",
			Host:      "upstream2.url",
			Port:      8080,
			Path:      "/api/v1",
			IssuerUrl: issuerUrl,
		},
	}
}

var _ = Describe("FeatureBuilder LoadBalancing", Ordered, func() {
	var ctx = context.Background()
	ctx = contextutil.WithEnv(ctx, "test")
	BeforeEach(func() {
		mockKc = mock.NewMockKongClient(mockCtrl)
	})

	Context("Applying and Creating", Ordered, func() {
		It("should apply the LoadBalancing feature isolated", func() {
			loadBalancingRoute := enableLoadBalancing(false)
			configureMocks(ctx, loadBalancingRoute)

			By("building the features")
			builder := buildLoadBalancingFeature(ctx, loadBalancingRoute, false)

			By("checking the jumper config")
			verifyJumperConfig(builder)

			By("checking the request-transformer plugin")
			verifyRequestTransformerPluginIsolated(builder)
		})

		It("should apply the LoadBalancing feature with LastMileSecurity for a real route", func() {
			loadBalancingRoute := enableLoadBalancing(false)
			configureMocks(ctx, loadBalancingRoute)

			By("building the features")
			builder := buildLoadBalancingFeature(ctx, loadBalancingRoute, true)

			By("checking the jumper config")
			verifyJumperConfig(builder)

			By("checking the request-transformer plugin")
			verifyRequestTransformerPluginRealRoute(builder)
		})

		It("should apply the LoadBalancing feature with LastMileSecurity for a proxy route", func() {
			loadBalancingRoute := enableLoadBalancing(true)
			configureMocks(ctx, loadBalancingRoute)

			By("building the features")
			builder := buildLoadBalancingFeature(ctx, loadBalancingRoute, true)

			By("checking the jumper config")
			verifyJumperConfig(builder)

			By("checking the request-transformer plugin")
			verifyRequestTransformerPluginProxyRoute(builder)
		})
	})

})

func verifyRequestTransformerPluginIsolated(builder *features.Builder) {
	rtPlugin, ok := builder.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
	Expect(ok).To(BeTrue())

	By("checking the request-transformer plugin config")
	// Expect(rtPlugin.Config.Append.Headers).To(BeNil()) TODO: why is this expected to be nil?
	Expect(rtPlugin.Config.Remove.Headers).To(BeNil())
}

func verifyRequestTransformerPluginRealRoute(builder *features.Builder) {
	rtPlugin, ok := builder.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
	Expect(ok).To(BeTrue())

	By("checking the request-transformer plugin config")
	Expect(rtPlugin.Config.Append.Headers.Contains("remote_api_url")).To(BeFalse())
	Expect(rtPlugin.Config.Append.Headers.Contains("api_base_path")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains("access_token_forwarding")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains(plugin.JumperConfigKey)).To(BeTrue())
	Expect(rtPlugin.Config.Remove.Headers.Contains("consumer-token")).To(BeTrue())
}

func verifyRequestTransformerPluginProxyRoute(builder *features.Builder) {
	rtPlugin, ok := builder.Plugins["request-transformer"].(*plugin.RequestTransformerPlugin)
	Expect(ok).To(BeTrue())

	By("checking the request-transformer plugin config")
	Expect(rtPlugin.Config.Append.Headers.Contains("issuer")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains("client_id")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains("client_secret")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains("remote_api_url")).To(BeTrue())
	Expect(rtPlugin.Config.Append.Headers.Contains(plugin.JumperConfigKey)).To(BeTrue())
	Expect(rtPlugin.Config.Remove.Headers).To(BeNil())
}

func verifyJumperConfig(builder *features.Builder) {
	jumperConfig := builder.JumperConfig()
	By("Checking that JumperConfig contains both upstreams with weights")
	Expect(jumperConfig).NotTo(BeNil())
	Expect(jumperConfig.LoadBalancing).NotTo(BeNil())
	Expect(jumperConfig.LoadBalancing.Servers).To(HaveLen(2))
	Expect(jumperConfig.LoadBalancing.Servers[0].Upstream).To(Equal("http://upstream.url:8080/api/v1"))
	Expect(jumperConfig.LoadBalancing.Servers[0].Weight).To(Equal(2))
	Expect(jumperConfig.LoadBalancing.Servers[1].Upstream).To(Equal("http://upstream2.url:8080/api/v1"))
	Expect(jumperConfig.LoadBalancing.Servers[1].Weight).To(Equal(1))
}

func buildLoadBalancingFeature(ctx context.Context, loadBalancingRoute *gatewayv1.Route, withLastMileSecurity bool) *features.Builder {
	builder := features.NewFeatureBuilder(mockKc, loadBalancingRoute, nil, realm, gateway)
	builder.EnableFeature(feature.InstanceLoadBalancingFeature)
	if withLastMileSecurity {
		builder.EnableFeature(feature.InstanceLastMileSecurityFeature)
	}
	err := builder.Build(ctx)
	Expect(err).ToNot(HaveOccurred())

	b, ok := builder.(*features.Builder)
	Expect(ok).To(BeTrue())
	return b
}

func enableLoadBalancing(isProxyRoute bool) *gatewayv1.Route {
	loadBalancingRoute := route.DeepCopy()
	loadBalancingRoute.Spec.PassThrough = false
	loadBalancingRoute.Spec.Upstreams = NewLoadBalancingUpstreams(isProxyRoute)

	return loadBalancingRoute
}

func configureMocks(ctx context.Context, loadBalancingRoute *gatewayv1.Route) {
	mockKc.EXPECT().CreateOrReplaceRoute(ctx, loadBalancingRoute, gomock.Any()).Return(nil).Times(1)
	mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
	mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}
