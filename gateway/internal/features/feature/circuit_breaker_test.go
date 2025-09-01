// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	"github.com/telekom/controlplane/gateway/internal/features/feature/config"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"go.uber.org/mock/gomock"
)

var _ = Describe("BasicAuthFeature", func() {

	It("should return the correct feature type", func() {
		Expect(feature.InstanceCircuitBreakerFeature.Name()).To(Equal(gatewayv1.FeatureTypeCircuitBreaker))
	})

	It("should have the correct priority", func() {
		Expect(feature.InstanceCircuitBreakerFeature.Priority()).To(Equal(98))
	})

	Context("with mocked feature builder", func() {
		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
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
							CircuitBreaker: false,
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
							CircuitBreaker: false,
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
							CircuitBreaker: true,
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
				mockKongClient = mock.NewMockKongClient(ctrl)
				mockKongAdminApi = mock.NewMockKongAdminApi(ctrl)
			})

			It("should return error if route is missing from feature builder", func() {
				mockFeatureBuilder.EXPECT().GetRoute().Return(nil, false).Times(1)

				err := feature.InstanceCircuitBreakerFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).Should(ContainSubstring("cannot find route"))
			})

			It("should create kong upstream and targets and update feature builder upstream value", func() {
				// Setup
				ctx := context.Background()
				ctx = contextutil.WithEnv(ctx, "test")
				route := &gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-route-name",
					},
					Spec: gatewayv1.RouteSpec{
						Traffic: gatewayv1.Traffic{
							CircuitBreaker: true,
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetKongClient().Return(mockKongClient).Times(1)
				mockKongClient.EXPECT().GetKongAdminApi().Return(mockKongAdminApi).Times(1)

				var setUpstreamArg *client.CustomUpstream
				mockFeatureBuilder.EXPECT().SetUpstream(gomock.Any()).Do(func(upstream *client.CustomUpstream) {
					setUpstreamArg = upstream
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
				mockKongAdminApi.EXPECT().UpsertUpstreamWithResponse(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(upsertUpstreamWithResponse_func).Times(1)

				// mock CreateTargetForUpstreamWithResponse
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
				mockKongAdminApi.EXPECT().CreateTargetForUpstreamWithResponse(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(createTargetForUpstreamWithResponse_func).Times(1)

				// Execute
				err := feature.InstanceCircuitBreakerFeature.Apply(ctx, mockFeatureBuilder)

				// Verify
				Expect(err).Should(Not(HaveOccurred()))
				// pointer vs non-pointer
				Expect(*setUpstreamArg).To(BeEquivalentTo(client.CustomUpstream{
					Host: "test-route-name",
				}))

				Expect(upsertUpstreamWithResponse_upstreamNameArg).To(Equal("test-route-name"))

				expectedUpstreamBody := createTestCreateUpstreamJSONRequestBody(ctx, "test-route-name")
				// pointer vs non-pointer
				Expect(upsertUpstreamWithResponse_upstreamBodyArg).To(Equal(*expectedUpstreamBody))
				Expect(route.GetUpstreamId()).To(Equal("kong_upstream_response_id"))

				Expect(createTargetForUpstreamWithResponse_upstreamNameArg).To(Equal("test-route-name"))
				expectedTargetTarget := "localhost:8080"
				expectedTargetWeight := 100
				Expect(createTargetForUpstreamWithResponse_targetBodyArg).To(Equal(kong.CreateTargetForUpstreamJSONRequestBody{
					Tags:   &[]string{"env--test", "targets--test-route-name"},
					Target: &expectedTargetTarget,
					Weight: &expectedTargetWeight,
				}))
				Expect(route.GetTargetsId()).To(Equal("kong_target_response_id"))
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
		},
	}
	return &upstreamBody
}
