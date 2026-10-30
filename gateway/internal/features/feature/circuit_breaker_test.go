// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CircuitBreakerFeature", func() {
	var (
		ctx     context.Context
		f       *feature.CircuitBreakerFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		f = feature.InstanceCircuitBreakerFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeCircuitBreaker", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeCircuitBreaker))
		})
	})

	Describe("Priority()", func() {
		It("returns 110", func() {
			Expect(f.Priority()).To(Equal(110))
		})
	})

	Describe("IsUsed()", func() {
		Context("when CircuitBreaker is enabled on a primary route", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when CircuitBreaker is disabled but upstreamId exists (cleanup scenario)", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "test-ns"},
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: false},
						},
					},
				}
				route.SetUpstreamId("existing-upstream-id")
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		DescribeTable("uses a disabled or removed CB for the cleanup if its upstream may exist",
			func(cb *gatewayv1.CircuitBreaker, prefix, targetsID string, expected bool) {
				route := &gatewayv1.Route{Spec: gatewayv1.RouteSpec{Type: gatewayv1.RouteTypePrimary}}
				route.SetNamePrefix(prefix)
				route.Spec.Traffic.CircuitBreaker = cb
				route.SetTargetsId(targetsID)
				builder.EXPECT().GetRoute().Return(route, true)
				Expect(f.IsUsed(ctx, builder)).To(Equal(expected))
			},
			Entry("removed, realm route", nil, "bond", "", true),
			Entry("disabled, realm route", &gatewayv1.CircuitBreaker{Enabled: false}, "bond", "", true),
			Entry("removed, targets ID only", nil, "", "t1", true),
			Entry("removed, plain route without IDs", nil, "", "", false),
		)

		Context("when CircuitBreaker is nil and no upstreamId", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type:    gatewayv1.RouteTypePrimary,
						Traffic: gatewayv1.Traffic{},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when route is a proxy route", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypeProxy,
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no route is in the builder", func() {
			It("returns false", func() {
				builder.EXPECT().GetRoute().Return(nil, false)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})
	})

	Describe("Apply()", func() {
		var route *gatewayv1.Route

		BeforeEach(func() {
			route = &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "test-ns"},
				Spec: gatewayv1.RouteSpec{
					Type:    gatewayv1.RouteTypePrimary,
					Traffic: gatewayv1.Traffic{CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true}},
				},
			}
		})

		DescribeTable("hands the upstream and target to the Kong client with the service upstream",
			func(prefix, kongName string, realmTags []string) {
				route.SetNamePrefix(prefix)
				var upstream client.Upstream
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) { upstream = u }).Return()

				Expect(f.Apply(ctx, builder)).To(Succeed())
				managed, ok := upstream.(*client.CircuitBreakerUpstream)
				Expect(ok).To(BeTrue())
				Expect(managed.GetHostname()).To(Equal(kongName))
				Expect(managed.Upstream.Name).To(Equal(kongName))
				Expect(managed.Upstream.Healthchecks).NotTo(BeNil())
				Expect(*managed.Upstream.Tags).To(ConsistOf(append([]string{"env--test-env", "upstream--" + kongName, "route--test-route"}, realmTags...)))
				Expect(*managed.Target.Target).To(Equal(feature.DefaultTargetsTarget))
				Expect(*managed.Target.Tags).To(ConsistOf(append([]string{"env--test-env", "targets--" + kongName, "route--test-route"}, realmTags...)))
			},
			Entry("with name prefix", "bond", "bond--test-route", []string{"realm--bond"}),
			Entry("without name prefix", "", "test-route", nil),
		)

		DescribeTable("hands an upstream removal to the Kong client if the CB is off",
			func(cb *gatewayv1.CircuitBreaker, current client.Upstream, expectedHost string) {
				route.Spec.Traffic.CircuitBreaker = cb
				var upstream client.Upstream
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetUpstream().Return(current)
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) { upstream = u }).Return()

				Expect(f.Apply(ctx, builder)).To(Succeed())
				removal, ok := upstream.(*client.UpstreamRemoval)
				Expect(ok).To(BeTrue())
				Expect(removal.GetHostname()).To(Equal(expectedHost))
			},
			Entry("disabled, no upstream yet", &gatewayv1.CircuitBreaker{Enabled: false}, nil, "localhost"),
			Entry("removed, keeps the upstream of another feature", nil, client.NewUpstreamOrDie("https://backend.example:443/api"), "backend.example"),
		)

		It("returns an error when no route is in the builder", func() {
			builder.EXPECT().GetRoute().Return(nil, false)
			Expect(f.Apply(ctx, builder)).To(MatchError(ContainSubstring("cannot find route")))
		})
	})
})
