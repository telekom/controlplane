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

	It("has the request termination feature type", func() {
		Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeRequestTermination))
	})

	DescribeTable("uses healthProbe to determine activation",
		func(healthProbe, hasRoute, wantUsed bool) {
			if hasRoute {
				builder.EXPECT().GetRoute().Return(&gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{Traffic: gatewayv1.Traffic{HealthProbe: healthProbe}},
				}, true)
			} else {
				builder.EXPECT().GetRoute().Return(nil, false)
			}
			Expect(f.IsUsed(ctx, builder)).To(Equal(wantUsed))
		},
		Entry("enabled", true, true, true),
		Entry("disabled", false, true, false),
		Entry("no route", true, false, false),
	)

	It("requests the request-termination plugin from the builder", func() {
		route := &gatewayv1.Route{
			Spec: gatewayv1.RouteSpec{Traffic: gatewayv1.Traffic{HealthProbe: true}},
		}
		builder.EXPECT().GetRoute().Return(route, true)
		builder.EXPECT().RequestTerminationPlugin().Return(&plugin.RequestTerminationPlugin{})
		Expect(f.Apply(ctx, builder)).To(Succeed())
	})

	It("returns ErrNoRoute when no route is available", func() {
		builder.EXPECT().GetRoute().Return(nil, false)
		Expect(f.Apply(ctx, builder)).To(MatchError(features.ErrNoRoute))
	})
})
