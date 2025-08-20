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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FeatureBuilder HeaderTransformation", Ordered, func() {
	var ctx = context.Background()
	ctx = contextutil.WithEnv(ctx, "test")
	BeforeEach(func() {
		mockKc = mock.NewMockKongClient(mockCtrl)
	})

	Context("Applying and Creating", Ordered, func() {

		BeforeEach(func() {
			mockKc = mock.NewMockKongClient(mockCtrl)
		})

		It("should apply the HeaderTransformation RT config", func() {
			route := NewMockRouteWithRemoveHeaders()

			By("building the features")
			builder := features.NewFeatureBuilder(mockKc, route, nil, realm, gateway)
			builder.SetUpstream(route.Spec.Upstreams[0])
			builder.EnableFeature(&feature.HeaderTransformationFeature{})

			mockKc.EXPECT().CreateOrReplaceRoute(ctx, route, gomock.Any(), gomock.AssignableToTypeOf(&gatewayv1.Gateway{})).Return(nil).Times(1)
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).Return(nil, nil).Times(1)
			mockKc.EXPECT().CleanupPlugins(ctx, gomock.Any(), nil, gomock.Any()).Return(nil).Times(1)

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
			Expect(rtPlugin.Config.Remove.Headers.Contains("X-Remove-Header"))

		})

	})

})

func NewMockRouteWithRemoveHeaders() *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: gatewayv1.RouteSpec{
			Realm: types.ObjectRef{
				Name:      "realm",
				Namespace: "default",
			},
			PassThrough: false,
			Transformation: &gatewayv1.Transformation{
				Request: gatewayv1.RequestResponseTransformation{
					Headers: gatewayv1.HeaderTransformation{
						Remove: []string{"X-Remove-Header"},
					},
				},
			},
			Upstreams: []gatewayv1.Upstream{
				{
					// Default is used for Weight
					Scheme: "http",
					Host:   "upstream.url",
					Port:   8080,
					Path:   "/api/v1",
				},
			},
			Downstreams: []gatewayv1.Downstream{
				{
					Host:      "downstream.url",
					Port:      8080,
					Path:      "/test/v1",
					IssuerUrl: "issuer.url",
				},
			},
		},
	}
}
