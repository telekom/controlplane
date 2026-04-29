// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"fmt"
	"net/http"

	testifymock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	"github.com/telekom/controlplane/gateway/internal/features/feature/config"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CircuitBreakerFeature", func() {
	It("should return the correct feature type", func() {
		Expect(feature.InstanceCircuitBreakerFeature.Name()).To(Equal(gatewayv1.FeatureTypeCircuitBreaker))
	})

	It("should have the correct priority", func() {
		Expect(feature.InstanceCircuitBreakerFeature.Priority()).To(Equal(110))
	})

	Context("with mocked feature builder", func() {
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder

		BeforeEach(func() {
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(GinkgoT())
		})

		Context("test IsUsed", func() {
			It("should not be used when route is not present in feature builder", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false).Times(1)

				Expect(feature.InstanceCircuitBreakerFeature.IsUsed(context.Background(), mockFeatureBuilder)).Should(BeFalse())
			})

			It("should not be used when circuit breaker is disabled in route spec", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: false,
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)

				Expect(feature.InstanceCircuitBreakerFeature.IsUsed(context.Background(), mockFeatureBuilder)).Should(BeFalse())
			})

			It("should be used when circuit breaker is disabled in route spec, but route property has upstream id", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: false,
							},
						},
					},
				}
				route.SetUpstreamId("someUpstreamId")
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)

				Expect(feature.InstanceCircuitBreakerFeature.IsUsed(context.Background(), mockFeatureBuilder)).Should(BeTrue())
			})

			It("should be used when CB is enabled in route spec", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: true,
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)

				Expect(feature.InstanceCircuitBreakerFeature.IsUsed(context.Background(), mockFeatureBuilder)).Should(BeTrue())
			})
		})

		Context("test Apply", func() {
			var mockKongClient *mock.MockKongClient
			var mockKongAdminApi *mock.MockKongAdminApi

			BeforeEach(func() {
				mockKongClient = mock.NewMockKongClient(GinkgoT())
				mockKongAdminApi = mock.NewMockKongAdminApi(GinkgoT())
			})

			It("should return error if route is missing from feature builder", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false).Times(1)

				err := feature.InstanceCircuitBreakerFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring("cannot find route"))
			})

			It("should create kong upstream and target on first reconciliation (target not found in Kong)", func() {
				// Setup
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-route-name",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: true,
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)

				var setUpstreamArg *client.CustomUpstream
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything).Run(func(upstream client.Upstream) {
					if cu, ok := upstream.(*client.CustomUpstream); ok {
						setUpstreamArg = cu
					}
				})

				// mock UpsertUpstreamWithResponse
				var upsertUpstreamWithResponse_upstreamNameArg string
				var upsertUpstreamWithResponse_upstreamBodyArg kong.CreateUpstreamJSONRequestBody
				upsertUpstreamWithResponse_func := func(_ context.Context, upstreamName string, upstreamBody kong.UpsertUpstreamJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.UpsertUpstreamResponse, error) {
					upsertUpstreamWithResponse_upstreamNameArg = upstreamName
					upsertUpstreamWithResponse_upstreamBodyArg = upstreamBody

					upsertUpstreamResponseId := "kong_upstream_response_id"
					return &kong.UpsertUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200: &kong.Upstream{
							Id: &upsertUpstreamResponseId,
						},
					}, nil
				}
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).RunAndReturn(upsertUpstreamWithResponse_func).Times(1)

				// mock FetchTargetForUpstreamWithResponse — target does not exist yet (404)
				mockKongAdminApi.EXPECT().FetchTargetForUpstreamWithResponse(testifymock.Anything, "test-route-name", "localhost:8080", testifymock.Anything).
					Return(&kong.FetchTargetForUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 404},
					}, nil).Times(1)

				// mock CreateTargetForUpstreamWithResponse — target is created
				var createTargetForUpstreamWithResponse_upstreamNameArg string
				var createTargetForUpstreamWithResponse_targetBodyArg kong.CreateTargetForUpstreamJSONRequestBody

				createTargetForUpstreamWithResponse_func := func(_ context.Context, upstreamName string, targetsBody kong.CreateTargetForUpstreamJSONRequestBody, _ ...kong.RequestEditorFn) (*kong.CreateTargetForUpstreamResponse, error) {
					createTargetForUpstreamWithResponse_upstreamNameArg = upstreamName
					createTargetForUpstreamWithResponse_targetBodyArg = targetsBody

					createTargetForUpstreamResponseId := "kong_target_response_id"
					return &kong.CreateTargetForUpstreamResponse{
						HTTPResponse: &http.Response{
							StatusCode: 200,
						},
						JSON200: &kong.Target{
							Id: &createTargetForUpstreamResponseId,
						},
					}, nil
				}
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).RunAndReturn(createTargetForUpstreamWithResponse_func).Times(1)

				// Execute
				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)

				// Verify
				Expect(err).Should(Not(HaveOccurred()))
				Expect(*setUpstreamArg).To(BeEquivalentTo(client.CustomUpstream{
					Scheme: "http",
					Host:   "test-route-name",
					Port:   8080,
					Path:   "/proxy",
				}))

				Expect(upsertUpstreamWithResponse_upstreamNameArg).To(Equal("test-route-name"))

				expectedUpstreamBody := createTestCreateUpstreamJSONRequestBody(ctx, "test-route-name")
				Expect(upsertUpstreamWithResponse_upstreamBodyArg).To(Equal(*expectedUpstreamBody))
				Expect(route.GetUpstreamId()).To(Equal("kong_upstream_response_id"))

				Expect(createTargetForUpstreamWithResponse_upstreamNameArg).To(Equal("test-route-name"))
				expectedTargetTarget := "localhost:8080"
				expectedTargetWeight := 100
				Expect(createTargetForUpstreamWithResponse_targetBodyArg).To(Equal(kong.CreateTargetForUpstreamJSONRequestBody{
					Tags:   &[]string{"env--test", "targets--test-route-name", "route--test-route-name"},
					Target: &expectedTargetTarget,
					Weight: &expectedTargetWeight,
				}))
				Expect(route.GetTargetsId()).To(Equal("kong_target_response_id"))
			})

			It("should upsert upstream but skip target creation when target already exists in Kong", func() {
				// Setup
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-route-name",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: true,
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything)

				// mock UpsertUpstreamWithResponse — still called on every reconciliation
				upsertUpstreamResponseId := "kong_upstream_response_id"
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.UpsertUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Upstream{Id: &upsertUpstreamResponseId},
					}, nil).Times(1)

				// mock FetchTargetForUpstreamWithResponse — target already exists in Kong
				existingTargetId := "existing-kong-target-id"
				mockKongAdminApi.EXPECT().FetchTargetForUpstreamWithResponse(testifymock.Anything, "test-route-name", "localhost:8080", testifymock.Anything).
					Return(&kong.FetchTargetForUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Target{Id: &existingTargetId},
					}, nil).Times(1)

				// CreateTargetForUpstreamWithResponse must NOT be called — target already exists
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

				// Execute
				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)

				// Verify
				Expect(err).Should(Not(HaveOccurred()))
				Expect(route.GetUpstreamId()).To(Equal("kong_upstream_response_id"))
				Expect(route.GetTargetsId()).To(Equal("existing-kong-target-id"))
			})

			It("should return error when FetchTargetForUpstream fails", func() {
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route-name"},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything)

				upsertUpstreamResponseId := "kong_upstream_response_id"
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.UpsertUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Upstream{Id: &upsertUpstreamResponseId},
					}, nil).Times(1)

				mockKongAdminApi.EXPECT().FetchTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(nil, fmt.Errorf("connection refused")).Times(1)

				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring("failed to fetch target for upstream"))
			})

			It("should create target when FetchTargetForUpstream returns 404 (new upstream)", func() {
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route-name"},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything)

				upsertUpstreamResponseId := "kong_upstream_response_id"
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.UpsertUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Upstream{Id: &upsertUpstreamResponseId},
					}, nil).Times(1)

				// FetchTargetForUpstream returns 404 — target doesn't exist yet
				mockKongAdminApi.EXPECT().FetchTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.FetchTargetForUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 404},
					}, nil).Times(1)

				// Since 404 means no target, CreateTargetForUpstream should be called
				createTargetResponseId := "new-target-id"
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.CreateTargetForUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Target{Id: &createTargetResponseId},
					}, nil).Times(1)

				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)
				Expect(err).Should(Not(HaveOccurred()))
				Expect(route.GetTargetsId()).To(Equal("new-target-id"))
			})

			It("should return error when CreateTargetForUpstream fails", func() {
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route-name"},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{Enabled: true},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything)

				upsertUpstreamResponseId := "kong_upstream_response_id"
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.UpsertUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 200},
						JSON200:      &kong.Upstream{Id: &upsertUpstreamResponseId},
					}, nil).Times(1)

				// Target does not exist
				mockKongAdminApi.EXPECT().FetchTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(&kong.FetchTargetForUpstreamResponse{
						HTTPResponse: &http.Response{StatusCode: 404},
					}, nil).Times(1)

				// CreateTargetForUpstream fails
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).
					Return(nil, fmt.Errorf("connection refused")).Times(1)

				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring("failed to create target for upstream"))
			})

			It("should delete kong upstream and targets if CB is disabled and upstreamId is not empty", func() {
				// Setup
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-route-name",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: &gatewayv1.CircuitBreaker{
								Enabled: false,
							},
						},
					},
					Status: gatewayv1.RouteStatus{
						Properties: map[string]string{"upstreamId": "kong_upstream_response_id"},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)

				var setUpstreamArg *client.CustomUpstream
				mockFeatureBuilder.EXPECT().SetUpstream(testifymock.Anything).Run(func(upstream client.Upstream) {
					if cu, ok := upstream.(*client.CustomUpstream); ok {
						setUpstreamArg = cu
					}
				})

				// mock UpsertUpstreamWithResponse
				var routeArg client.CustomRoute
				deleteUpstream_func := func(_ context.Context, route client.CustomRoute) error {
					routeArg = route

					// happy path
					return nil
				}
				mockKongClient.EXPECT().DeleteUpstream(testifymock.Anything, testifymock.Anything).RunAndReturn(deleteUpstream_func).Times(1)

				// Execute
				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)

				// Verify
				Expect(err).Should(Not(HaveOccurred()))
				// pointer vs non-pointer
				Expect(*setUpstreamArg).To(BeEquivalentTo(client.CustomUpstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   8080,
					Path:   "/proxy",
				}))

				Expect(routeArg.GetName()).To(Equal(route.GetName()))
				gwRoute, ok := routeArg.(*gatewayv1.Route)
				Expect(ok).To(BeTrue())
				Expect(gwRoute.GetUpstreamId()).To(Equal(""))
			})
		})
	})
})

func createTestCreateUpstreamJSONRequestBody(ctx context.Context, upstreamName string) *kong.CreateUpstreamJSONRequestBody {
	upstreamAlgorithm := kong.RoundRobin
	passiveHealthcheckType := kong.CreateUpstreamRequestHealthchecksPassiveTypeHttp
	activeHealthcheckType := kong.CreateUpstreamRequestHealthchecksActiveTypeHttp

	upstreamBody := kong.CreateUpstreamJSONRequestBody{
		Algorithm: &upstreamAlgorithm,
		Name:      upstreamName,
		Healthchecks: &kong.CreateUpstreamRequestHealthchecks{
			Active: &kong.CreateUpstreamRequestHealthchecksActive{
				Healthy: &kong.CreateUpstreamRequestHealthchecksActiveHealthy{
					HttpStatuses: &config.DefaultCircuitBreaker.Active.HealthyHttpStatuses,
				},
				Type: &activeHealthcheckType,
				Unhealthy: &kong.CreateUpstreamRequestHealthchecksActiveUnhealthy{
					HttpStatuses: &config.DefaultCircuitBreaker.Active.UnhealthyHttpStatuses,
				},
			},
			Passive: &kong.CreateUpstreamRequestHealthchecksPassive{
				Healthy: &kong.CreateUpstreamRequestHealthchecksPassiveHealthy{
					HttpStatuses: config.ToPassiveHealthyHttpStatuses(config.DefaultCircuitBreaker.Passive.HealthyHttpStatuses),
					Successes:    &config.DefaultCircuitBreaker.Passive.HealthySuccesses,
				},
				Type: &passiveHealthcheckType,
				Unhealthy: &kong.CreateUpstreamRequestHealthchecksPassiveUnhealthy{
					HttpFailures: &config.DefaultCircuitBreaker.Passive.UnhealthyHttpFailures,
					HttpStatuses: config.ToPassiveUnhealthyHttpStatuses(config.DefaultCircuitBreaker.Passive.UnhealthyHttpStatuses),
					TcpFailures:  &config.DefaultCircuitBreaker.Passive.UnhealthyTcpFailures,
					Timeouts:     &config.DefaultCircuitBreaker.Passive.UnhealthyTimeouts,
				},
			},
		},
		Tags: &[]string{
			client.BuildTag("env", contextutil.EnvFromContextOrDie(ctx)),
			client.BuildTag("upstream", upstreamName),
			client.BuildTag("route", "test-route-name"),
		},
	}
	return &upstreamBody
}
