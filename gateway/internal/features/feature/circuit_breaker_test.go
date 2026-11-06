// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CircuitBreakerFeature", func() {
	var (
		ctx     context.Context
		f       *feature.CircuitBreakerFeature
		builder *featmock.MockFeaturesBuilder
		route   *gatewayv1.Route
	)

	BeforeEach(func() {
		ctx = contextutil.WithEnv(context.Background(), "test-env")
		f = feature.InstanceCircuitBreakerFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
		route = &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "test-ns"},
			Spec: gatewayv1.RouteSpec{
				Type:    gatewayv1.RouteTypePrimary,
				Traffic: gatewayv1.Traffic{CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true}},
			},
		}
	})

	It("has the circuit-breaker name and highest feature priority", func() {
		Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeCircuitBreaker))
		Expect(f.Priority()).To(Equal(110))
	})

	Describe("IsUsed()", func() {
		DescribeTable("applies enabled primary breakers and cleans up potentially existing upstreams",
			func(routeType gatewayv1.RouteType, cb *gatewayv1.CircuitBreaker, prefix, upstreamID, targetsID string, expected bool) {
				route.Spec.Type = routeType
				route.Spec.Traffic.CircuitBreaker = cb
				route.SetNamePrefix(prefix)
				route.SetUpstreamId(upstreamID)
				route.SetTargetsId(targetsID)
				builder.EXPECT().GetRoute().Return(route, true)
				Expect(f.IsUsed(ctx, builder)).To(Equal(expected))
			},
			Entry("enabled primary", gatewayv1.RouteTypePrimary, &gatewayv1.CircuitBreaker{Enabled: true}, "", "", "", true),
			Entry("disabled with stored ID", gatewayv1.RouteTypePrimary, &gatewayv1.CircuitBreaker{Enabled: false}, "", "u1", "", true),
			Entry("removed realm breaker", gatewayv1.RouteTypePrimary, nil, "bond", "", "", true),
			Entry("disabled realm breaker", gatewayv1.RouteTypePrimary, &gatewayv1.CircuitBreaker{Enabled: false}, "bond", "", "", true),
			Entry("target ID only", gatewayv1.RouteTypePrimary, nil, "", "", "t1", true),
			Entry("removed without IDs", gatewayv1.RouteTypePrimary, nil, "", "", "", false),
			Entry("proxy without IDs", gatewayv1.RouteTypeProxy, &gatewayv1.CircuitBreaker{Enabled: true}, "", "", "", false),
			Entry("proxy with stored ID", gatewayv1.RouteTypeProxy, &gatewayv1.CircuitBreaker{Enabled: true}, "", "u1", "", true),
			Entry("proxy with lost IDs and realm discovery", gatewayv1.RouteTypeProxy, &gatewayv1.CircuitBreaker{Enabled: true}, "bond", "", "", true),
		)

		It("returns false without a route", func() {
			builder.EXPECT().GetRoute().Return(nil, false)
			Expect(f.IsUsed(ctx, builder)).To(BeFalse())
		})
	})

	Describe("Apply()", func() {
		DescribeTable("hands the upstream and target to the Kong client with the service upstream",
			func(prefix, kongName string, realmTags []string, listeners []*gatewayv1.RouteListener, path string) {
				route.SetNamePrefix(prefix)
				var upstream client.Upstream
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetRouteListeners().Return(listeners)
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) { upstream = u }).Return()

				Expect(f.Apply(ctx, builder)).To(Succeed())
				managed, ok := upstream.(*client.CircuitBreakerUpstream)
				Expect(ok).To(BeTrue())
				Expect(managed.GetHostname()).To(Equal(kongName))
				Expect(managed.GetPort()).To(Equal(8080))
				Expect(managed.GetPath()).To(Equal(path))
				Expect(managed.Upstream.Name).To(Equal(kongName))
				Expect(managed.Upstream.Healthchecks).NotTo(BeNil())
				Expect(*managed.Upstream.Tags).To(ConsistOf(append([]string{"env--test-env", "upstream--" + kongName, "route--test-route"}, realmTags...)))
				Expect(*managed.Target.Target).To(Equal(feature.DefaultTargetsTarget))
				Expect(*managed.Target.Tags).To(ConsistOf(append([]string{"env--test-env", "targets--" + kongName, "route--test-route"}, realmTags...)))
			},
			Entry("with name prefix", "bond", "bond--test-route", []string{"realm--bond"}, nil, "/proxy"),
			Entry("without name prefix", "", "test-route", nil, nil, "/proxy"),
			Entry("with listener and name prefix", "bond", "bond--test-route", []string{"realm--bond"}, []*gatewayv1.RouteListener{{}}, "/listener"),
		)

		DescribeTable("hands an upstream removal to the Kong client if the CB is off",
			func(cb *gatewayv1.CircuitBreaker, current client.Upstream, listeners []*gatewayv1.RouteListener, host, path string) {
				route.Spec.Traffic.CircuitBreaker = cb
				var upstream client.Upstream
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().GetUpstream().Return(current)
				if current == nil {
					builder.EXPECT().GetRouteListeners().Return(listeners)
				}
				builder.EXPECT().SetUpstream(mock.Anything).Run(func(u client.Upstream) { upstream = u }).Return()

				Expect(f.Apply(ctx, builder)).To(Succeed())
				removal, ok := upstream.(*client.UpstreamRemoval)
				Expect(ok).To(BeTrue())
				Expect(removal.GetHostname()).To(Equal(host))
				Expect(removal.GetPath()).To(Equal(path))
			},
			Entry("disabled, no upstream yet", &gatewayv1.CircuitBreaker{Enabled: false}, nil, nil, "localhost", "/proxy"),
			Entry("removed, keeps another feature's upstream", nil, client.NewUpstreamOrDie("https://backend.example:443/api"), nil, "backend.example", "/api"),
			Entry("removed, keeps listener upstream", nil, client.NewUpstreamOrDie(plugin.LocalhostListenerUrl), []*gatewayv1.RouteListener{{}}, "localhost", "/listener"),
			Entry("disabled, listener fallback", &gatewayv1.CircuitBreaker{Enabled: false}, nil, []*gatewayv1.RouteListener{{}}, "localhost", "/listener"),
		)

		It("returns an error when no route is in the builder", func() {
			builder.EXPECT().GetRoute().Return(nil, false)
			Expect(f.Apply(ctx, builder)).To(MatchError(ContainSubstring("cannot find route")))
		})
	})

	Describe("lifecycle through the feature builder", func() {
		var mockKongClient *clientmock.MockKongClient

		build := func(listeners ...*gatewayv1.RouteListener) error {
			b := features.NewFeatureBuilder(mockKongClient, route, nil, &gatewayv1.Gateway{})
			b.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
			b.EnableFeature(feature.InstanceRouteListenerFeature)
			b.EnableFeature(feature.InstanceCircuitBreakerFeature)
			b.AddRouteListeners(listeners...)
			return b.Build(ctx)
		}

		BeforeEach(func() {
			mockKongClient = clientmock.NewMockKongClient(GinkgoT())
			mockKongClient.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		})

		DescribeTable("deletes the Kong upstream once the circuit breaker is no longer wanted",
			func(routeType gatewayv1.RouteType, cb *gatewayv1.CircuitBreaker) {
				mockKongClient.EXPECT().CreateOrReplaceRoute(mock.Anything, route, mock.AnythingOfType("*client.CircuitBreakerUpstream")).
					Run(func(_ context.Context, r client.CustomRoute, _ client.Upstream) {
						r.SetUpstreamId("upstream-id")
						r.SetTargetsId("target-id")
					}).Return(nil).Once()
				Expect(build()).To(Succeed())
				Expect(route.GetUpstreamId()).To(Equal("upstream-id"))
				Expect(route.GetTargetsId()).To(Equal("target-id"))

				route.Spec.Type = routeType
				route.Spec.Traffic.CircuitBreaker = cb
				mockKongClient.EXPECT().CreateOrReplaceRoute(mock.Anything, route, mock.AnythingOfType("*client.UpstreamRemoval")).
					Run(func(_ context.Context, r client.CustomRoute, _ client.Upstream) {
						Expect(route.GetUpstreamId()).To(Equal("upstream-id"))
						r.SetUpstreamId("")
						r.SetTargetsId("")
					}).Return(nil).Once()
				Expect(build()).To(Succeed())
				Expect(route.Status.Properties).NotTo(HaveKey("upstreamId"))
				Expect(route.Status.Properties).NotTo(HaveKey("targetsId"))
				Expect(f.IsUsed(ctx, features.NewFeatureBuilder(mockKongClient, route, nil, &gatewayv1.Gateway{}))).To(BeFalse())
			},
			Entry("when disabled", gatewayv1.RouteTypePrimary, &gatewayv1.CircuitBreaker{Enabled: false}),
			Entry("when removed", gatewayv1.RouteTypePrimary, nil),
			Entry("when the route becomes a proxy", gatewayv1.RouteTypeProxy, nil),
			Entry("when the route becomes a failover secondary", gatewayv1.RouteTypeSecondary, nil),
			Entry("when a proxy still has the breaker enabled", gatewayv1.RouteTypeProxy, &gatewayv1.CircuitBreaker{Enabled: true}),
		)

		DescribeTable("keeps the jumper /listener path with realm naming",
			func(enabled bool) {
				route.SetNamePrefix("bond")
				route.Spec.Traffic.CircuitBreaker.Enabled = enabled
				rl := &gatewayv1.RouteListener{
					ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
					Spec:       gatewayv1.RouteListenerSpec{Consumer: "consumer-app", ServiceOwner: "provider-app", Issue: "/api/v1/events"},
				}
				var receivedUpstream client.Upstream
				mockKongClient.EXPECT().CreateOrReplaceRoute(mock.Anything, route, mock.Anything).
					Run(func(_ context.Context, _ client.CustomRoute, u client.Upstream) { receivedUpstream = u }).Return(nil)
				mockKongClient.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).Return(nil, nil)

				Expect(build(rl)).To(Succeed())
				Expect(receivedUpstream.GetPath()).To(Equal("/listener"))
				Expect(receivedUpstream.GetPort()).To(Equal(8080))
				if enabled {
					Expect(receivedUpstream.GetHostname()).To(Equal("bond--test-route"))
				} else {
					Expect(receivedUpstream.GetHostname()).To(Equal("localhost"))
				}
			},
			Entry("enabled", true),
			Entry("disabled", false),
		)

		DescribeTable("returns Kong errors and retains IDs for retry",
			func(enabled bool) {
				route.Spec.Traffic.CircuitBreaker.Enabled = enabled
				route.SetUpstreamId("upstream-id")
				route.SetTargetsId("target-id")
				mockKongClient.EXPECT().CreateOrReplaceRoute(mock.Anything, route, mock.Anything).Return(errors.New("connection refused"))
				Expect(build()).To(MatchError(ContainSubstring("connection refused")))
				Expect(route.GetUpstreamId()).To(Equal("upstream-id"))
				Expect(route.GetTargetsId()).To(Equal("target-id"))
			},
			Entry("upstream reconciliation", true),
			Entry("upstream removal", false),
		)
	})
})
