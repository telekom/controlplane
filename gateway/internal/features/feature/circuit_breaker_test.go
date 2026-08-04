// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		Context("apply scenario - CircuitBreaker enabled", func() {
			var (
				route          *gatewayv1.Route
				mockKongClient *clientmock.MockKongClient
			)

			BeforeEach(func() {
				route = &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "test-ns"},
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				mockKongClient = clientmock.NewMockKongClient(GinkgoT())
			})

			It("delegates upstream reconciliation to the Kong client", func() {
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().CreateOrReplaceUpstream(
					mock.Anything,
					route,
					mock.MatchedBy(func(body *kong.CreateUpstreamJSONRequestBody) bool {
						return body.Name == route.Name && body.Healthchecks != nil
					}),
					mock.MatchedBy(func(body *kong.CreateTargetForUpstreamJSONRequestBody) bool {
						return body.Target != nil && *body.Target == feature.DefaultTargetsTarget
					}),
				).Return(nil)

				Expect(f.Apply(ctx, builder)).To(Succeed())
			})

			It("returns a wrapped Kong client error", func() {
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().CreateOrReplaceUpstream(mock.Anything, route, mock.Anything, mock.Anything).
					Return(errors.New("connection refused"))

				err := f.Apply(ctx, builder)
				Expect(err).To(MatchError("failed to create or replace upstream: connection refused"))
			})
		})

		Context("delete scenario - CircuitBreaker disabled but upstreamId present", func() {
			var (
				route          *gatewayv1.Route
				mockKongClient *clientmock.MockKongClient
			)

			BeforeEach(func() {
				route = &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "test-ns"},
					Spec: gatewayv1.RouteSpec{
						Type: gatewayv1.RouteTypePrimary,
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: false},
						},
					},
				}
				route.SetUpstreamId("existing-upstream-id")
				route.SetTargetsId("existing-target-id")
				mockKongClient = clientmock.NewMockKongClient(GinkgoT())
			})

			It("deletes the upstream and clears upstreamId and targetsId", func() {
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().DeleteUpstream(mock.Anything, mock.Anything).Return(nil)

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())
				Expect(route.GetUpstreamId()).To(BeEmpty())
				Expect(route.GetTargetsId()).To(BeEmpty())
			})

			It("returns the error when DeleteUpstream fails", func() {
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().DeleteUpstream(mock.Anything, mock.Anything).Return(errors.New("upstream not found"))

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("upstream not found"))
			})
		})

		Context("error handling", func() {
			It("returns an error when no route is in the builder", func() {
				builder.EXPECT().GetRoute().Return(nil, false)

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("cannot find route"))
			})
		})
	})
})
