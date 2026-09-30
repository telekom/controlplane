// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RequestTerminationFeature", func() {
	var (
		ctx     context.Context
		f       *feature.RequestTerminationFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceRequestTerminationFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	It("has the RequestTermination name and priority 0", func() {
		Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeRequestTermination))
		Expect(f.Priority()).To(Equal(0))
	})

	Describe("IsUsed()", func() {
		It("returns true when request termination is set", func() {
			route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{
				RequestTermination: &gatewayv1.RequestTermination{StatusCode: 200},
			}}
			builder.EXPECT().GetRoute().Return(route, true)

			Expect(f.IsUsed(ctx, builder)).To(BeTrue())
		})

		It("returns false when request termination is not set", func() {
			builder.EXPECT().GetRoute().Return(&gatewayv1.Route{}, true)

			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})

		It("returns false when there is no route", func() {
			builder.EXPECT().GetRoute().Return(nil, false)

			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})
	})

	Describe("Apply()", func() {
		It("configures the request-termination plugin with the status code", func() {
			route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{
				RequestTermination: &gatewayv1.RequestTermination{StatusCode: 200},
			}}
			route.Name = "zone-health"
			p := plugin.RequestTerminationPluginFromRoute(route)
			builder.EXPECT().GetRoute().Return(route, true)
			builder.EXPECT().RequestTerminationPlugin().Return(p)

			Expect(f.Apply(ctx, builder)).To(Succeed())
			Expect(p.GetName()).To(Equal("request-termination"))
			Expect(*p.GetRoute()).To(Equal("zone-health"))
			Expect(p.GetConsumer()).To(BeNil())
			Expect(p.GetConfig()).To(Equal(map[string]any{"status_code": 200}))
		})

		It("returns ErrNoRoute when there is no route", func() {
			builder.EXPECT().GetRoute().Return(nil, false)

			Expect(f.Apply(ctx, builder)).To(MatchError(features.ErrNoRoute))
		})
	})
})
