// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"errors"
	"net/http"

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
				route            *gatewayv1.Route
				mockKongClient   *clientmock.MockKongClient
				mockKongAdminApi *clientmock.MockKongAdminApi
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
				mockKongAdminApi = clientmock.NewMockKongAdminApi(GinkgoT())
			})

			It("upserts upstream and creates target, storing IDs on the route", func() {
				upstreamId := "upstream-123"
				upstreamResp := &kong.UpsertUpstreamResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &kong.Upstream{Id: &upstreamId},
				}

				targetId := "target-456"
				targetResp := &kong.CreateTargetForUpstreamResponse{
					HTTPResponse: &http.Response{StatusCode: 201},
					JSON200:      &kong.Target{Id: &targetId},
				}

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi)
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(upstreamResp, nil)
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).Return(targetResp, nil)

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())
				Expect(route.GetUpstreamId()).To(Equal("upstream-123"))
				Expect(route.GetTargetsId()).To(Equal("target-456"))
			})

			It("returns a wrapped error when UpsertUpstreamWithResponse fails", func() {
				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi)
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(nil, errors.New("connection refused"))

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create upstream"))
				Expect(err.Error()).To(ContainSubstring("connection refused"))
			})

			It("returns an error when UpsertUpstreamWithResponse returns a non-200 status", func() {
				upstreamResp := &kong.UpsertUpstreamResponse{
					Body:         []byte(`{"message":"bad request"}`),
					HTTPResponse: &http.Response{StatusCode: 400},
				}

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi)
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(upstreamResp, nil)

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create upstream"))
				Expect(err.Error()).To(ContainSubstring("400"))
			})

			It("returns a wrapped error when CreateTargetForUpstreamWithResponse fails", func() {
				upstreamId := "upstream-123"
				upstreamResp := &kong.UpsertUpstreamResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &kong.Upstream{Id: &upstreamId},
				}

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi)
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(upstreamResp, nil)
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(nil, errors.New("timeout"))

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create targets for upstream"))
				Expect(err.Error()).To(ContainSubstring("timeout"))
			})

			It("returns an error when CreateTargetForUpstreamWithResponse returns a non-200/201 status", func() {
				upstreamId := "upstream-123"
				upstreamResp := &kong.UpsertUpstreamResponse{
					HTTPResponse: &http.Response{StatusCode: 200},
					JSON200:      &kong.Upstream{Id: &upstreamId},
				}
				targetResp := &kong.CreateTargetForUpstreamResponse{
					Body:         []byte(`{"message":"conflict"}`),
					HTTPResponse: &http.Response{StatusCode: 409},
				}

				builder.EXPECT().GetRoute().Return(route, true)
				builder.EXPECT().SetUpstream(mock.Anything).Return()
				builder.EXPECT().GetKongClient().Return(mockKongClient)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi)
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(upstreamResp, nil)
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(mock.Anything, "test-route", mock.Anything).
					Return(targetResp, nil)

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create targets for upstream"))
				Expect(err.Error()).To(ContainSubstring("409"))
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
