// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper functions for creating test resources with rate limits

// NewApiExposureWithRateLimit creates an ApiExposure with both provider and consumer rate limits
func NewApiExposureWithRateLimit(apiBasePath, zoneName, consumerClientId string, appName string) *apiapi.ApiExposure {
	return &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiBasePath),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiapi.BasePathLabelKey:    labelutil.NormalizeValue(apiBasePath),
				util.ApplicationLabelKey:   labelutil.NormalizeValue(appName),
			},
		},
		Spec: apiapi.ApiExposureSpec{
			ApiBasePath: apiBasePath,
			Upstreams: []apiapi.Upstream{
				{
					Url:    "http://my-provider-api:8080/api/v1",
					Weight: 100,
				},
			},
			Traffic: apiapi.Traffic{
				RateLimit: &apiapi.RateLimit{
					Provider: &apiapi.RateLimitConfig{
						Limits: apiapi.Limits{
							Second: 100,
							Minute: 1000,
							Hour:   10000,
						},
						Options: apiapi.RateLimitOptions{
							HideClientHeaders: false,
							FaultTolerant:     true,
						},
					},
					SubscriberRateLimit: &apiapi.SubscriberRateLimits{
						Default: &apiapi.SubscriberRateLimitDefaults{
							Limits: apiapi.Limits{
								Second: 10,
								Minute: 100,
								Hour:   1000,
							},
						},
						Overrides: []apiapi.RateLimitOverrides{
							{
								Subscriber: consumerClientId,
								Limits: apiapi.Limits{
									Second: 20,
									Minute: 200,
									Hour:   2000,
								},
							},
						},
					},
				},
			},
			Security: &apiapi.Security{
				M2M: &apiapi.Machine2MachineAuthentication{
					ExternalIDP: &apiapi.ExternalIdentityProvider{
						TokenEndpoint: "https://example.com/token",
						TokenRequest:  "header",
						GrantType:     "client_credentials",
						Client: &apiapi.OAuth2ClientCredentials{
							ClientId:  "client-id",
							ClientKey: "******",
						},
					},
					Scopes: []string{"scope1"},
				},
			},
			Visibility: apiapi.VisibilityWorld,
			Approval: apiapi.Approval{
				Strategy: apiapi.ApprovalStrategyAuto,
			},
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
	}
}

// NewApiExposureWithConsumerOnlyRateLimit creates an ApiExposure with only consumer rate limits (no provider rate limits)
func NewApiExposureWithConsumerOnlyRateLimit(apiBasePath, zoneName, consumerClientId string, appName string) *apiapi.ApiExposure {
	apiExposure := NewApiExposureWithRateLimit(apiBasePath, zoneName, consumerClientId, appName)
	apiExposure.Spec.Traffic.RateLimit.Provider = nil
	return apiExposure
}

// NewApiExposureWithProviderOnlyRateLimit creates an ApiExposure with only provider rate limits (no consumer rate limits)
func NewApiExposureWithProviderOnlyRateLimit(apiBasePath, zoneName string, appName string) *apiapi.ApiExposure {
	apiExposure := NewApiExposureWithRateLimit(apiBasePath, zoneName, "", appName)
	apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit = nil
	return apiExposure
}

// createAndApproveSubscription creates an ApiSubscription and approves it
func createAndApproveSubscription(apiBasePath, zoneName, appName string, zoneRef *types.ObjectRef) *apiapi.ApiSubscription {
	// Create subscription
	subscription := NewApiSubscription(apiBasePath, zoneName, appName)

	// Set zone if provided
	if zoneRef != nil {
		subscription.Spec.Zone = *zoneRef
	}

	err := k8sClient.Create(ctx, subscription)
	Expect(err).ToNot(HaveOccurred())

	// Wait for approval request
	Eventually(func(g Gomega) {
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
		g.Expect(err).ToNot(HaveOccurred())
		testutil.ExpectConditionToBeFalse(g, meta.FindStatusCondition(subscription.GetConditions(), condition.ConditionTypeReady), "ApprovalPending")

		g.Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())
	}, timeout, interval).Should(Succeed())

	// Approve subscription
	approvalRequest := &approvalapi.ApprovalRequest{}
	err = k8sClient.Get(ctx, subscription.Status.ApprovalRequest.K8s(), approvalRequest)
	Expect(err).ToNot(HaveOccurred())

	approvalRequest = ProgressApprovalRequest(subscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
	ProgressApproval(subscription, approvalapi.ApprovalStateGranted, approvalRequest)

	return subscription
}

// verifyConsumeRouteRateLimits verifies that a ConsumeRoute has the expected rate limits
func verifyConsumeRouteRateLimits(subscription *apiapi.ApiSubscription, expectedLimits apiapi.Limits) {
	Eventually(func(g Gomega) {
		// Refresh subscription
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(subscription.Status.ConsumeRoute).ToNot(BeNil())

		// Get the ConsumeRoute
		consumeRoute := &gatewayapi.ConsumeRoute{}
		err = k8sClient.Get(ctx, subscription.Status.ConsumeRoute.K8s(), consumeRoute)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify rate limit configuration
		g.Expect(consumeRoute.Spec.Traffic.RateLimit).ToNot(BeNil(), "ConsumeRoute should have rate limit configuration")

		// Verify rate limit values
		g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).To(Equal(expectedLimits.Second))
		g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(expectedLimits.Minute))
		g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(expectedLimits.Hour))
	}, timeout, interval).Should(Succeed())
}

// verifyRouteRateLimits verifies that a Route has the expected provider rate limits
func verifyRouteRateLimits(route *types.ObjectRef, providerRateLimit *apiapi.RateLimitConfig, isProxyRoute bool) {
	Eventually(func(g Gomega) {
		// Get the Route
		routeObj := &gatewayapi.Route{}
		err := k8sClient.Get(ctx, route.K8s(), routeObj)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify rate limit configuration
		g.Expect(routeObj.Spec.Traffic.RateLimit).ToNot(BeNil(), "Route should have rate limit configuration")

		// Verify rate limit values
		g.Expect(routeObj.Spec.Traffic.RateLimit.Limits.Second).To(Equal(providerRateLimit.Limits.Second))
		g.Expect(routeObj.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(providerRateLimit.Limits.Minute))
		g.Expect(routeObj.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(providerRateLimit.Limits.Hour))

		// Verify rate limit options
		g.Expect(routeObj.Spec.Traffic.RateLimit.Options.HideClientHeaders).To(Equal(providerRateLimit.Options.HideClientHeaders))
		g.Expect(routeObj.Spec.Traffic.RateLimit.Options.FaultTolerant).To(Equal(providerRateLimit.Options.FaultTolerant))

		// Verify proxy route labeling
		if isProxyRoute {
			g.Expect(routeObj.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
		} else {
			g.Expect(routeObj.Labels[config.BuildLabelKey("type")]).ToNot(Equal("proxy"))
		}
	}, timeout, interval).Should(Succeed())
}

var _ = Describe("ApiSubscription Rate Limiting", Ordered, func() {
	// API that is used for the tests
	var apiBasePath = "/ratelimit/test/v1"

	// Provider side
	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure

	// Provider/Exposure zone
	var zoneName = "ratelimit-test"
	var secondZoneName = "ratelimit-test-2"
	var zone *adminapi.Zone
	var secondZone *adminapi.Zone

	// Consumer side
	var appName = "rate-limit-app"
	var application *applicationapi.Application
	var apiSubscription *apiapi.ApiSubscription

	// Second consumer for default rate limit test
	var defaultAppName = "default-rate-limit-app"
	var defaultApplication *applicationapi.Application
	var defaultApiSubscription *apiapi.ApiSubscription

	var apiExpAppName = "api-exposure-app"
	var apiExpApplication *applicationapi.Application

	BeforeAll(func() {
		By("Creating the Zones")
		zone = CreateZone(zoneName)
		CreateGatewayClient(zone)
		secondZone = CreateZone(secondZoneName)
		CreateGatewayClient(secondZone)

		By("Creating the Realms")
		CreateRealm(testEnvironment, zone.Name)
		CreateRealm(testEnvironment, secondZone.Name)

		By("Creating the Application for ApiExposure")
		apiExpApplication = CreateApplication(apiExpAppName)

		By("Creating the Applications")
		application = CreateApplication(appName)
		defaultApplication = CreateApplication(defaultAppName)
		defaultApplication.Status.ClientId = "default-test-client-id"
		err := k8sClient.Status().Update(ctx, defaultApplication)
		Expect(err).ToNot(HaveOccurred())

		By("Initializing the API")
		api = NewApi(apiBasePath)
		err = k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the APIExposure with rate limits")
		apiExposure = NewApiExposureWithRateLimit(apiBasePath, zoneName, application.Status.ClientId, apiExpAppName)
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())
			testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(apiExposure.GetConditions(), condition.ConditionTypeReady), "Provisioned")
			g.Expect(apiExposure.Status.Active).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		By("Cleaning up and deleting all resources")
		err := k8sClient.Delete(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		By("Deleting the Application for ApiExposure")
		err = k8sClient.Delete(ctx, apiExpApplication)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, application)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, defaultApplication)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Rate Limit Propagation Tests", func() {
		It("should pass consumer-specific rate limit configuration to ConsumeRoute", func() {
			By("Creating and approving the ApiSubscription")
			apiSubscription = createAndApproveSubscription(apiBasePath, zoneName, appName, nil)

			By("Verifying the ConsumeRoute has the consumer-specific rate limit")
			consumerRateLimit, ok := apiExposure.GetOverriddenSubscriberRateLimit(application.Status.ClientId)
			Expect(ok).To(BeTrue(), "ApiExposure should have a rate limit override for this consumer")
			verifyConsumeRouteRateLimits(apiSubscription, consumerRateLimit)
		})

		It("should pass default rate limit configuration to ConsumeRoute when no consumer-specific override exists", func() {
			By("Creating and approving the ApiSubscription for the default application")
			defaultApiSubscription = createAndApproveSubscription(apiBasePath, zoneName, defaultAppName, nil)

			By("Verifying the ConsumeRoute has the default rate limit")
			defaultRateLimit := apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default.Limits
			verifyConsumeRouteRateLimits(defaultApiSubscription, defaultRateLimit)

			// Verify no consumer-specific override exists for this application
			_, ok := apiExposure.GetOverriddenSubscriberRateLimit(defaultApplication.Status.ClientId)
			Expect(ok).To(BeFalse(), "ApiExposure should not have a rate limit override for this consumer")
		})

		It("should pass provider rate limiting configuration to routes", func() {
			By("Verifying the Route is created with provider rate limits")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify regular route has correct provider rate limits
			verifyRouteRateLimits(apiExposure.Status.Route, apiExposure.Spec.Traffic.RateLimit.Provider, false)

			By("Creating a subscription in a different zone to test proxy route")
			proxyAppName := "proxy-app"
			proxyApplication := CreateApplication(proxyAppName)
			proxyApplication.Status.ClientId = "proxy-client-id"
			err := k8sClient.Status().Update(ctx, proxyApplication)
			Expect(err).ToNot(HaveOccurred())

			// Create and approve subscription in second zone
			zoneRef := &types.ObjectRef{
				Name:      secondZoneName,
				Namespace: testEnvironment,
			}
			proxyApiSubscription := createAndApproveSubscription(apiBasePath, secondZoneName, proxyAppName, zoneRef)

			By("Verifying the ProxyRoute is created with provider rate limits")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(proxyApiSubscription), proxyApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(proxyApiSubscription.Status.Route).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify proxy route has correct provider rate limits and is labeled as proxy
			verifyRouteRateLimits(proxyApiSubscription.Status.Route, apiExposure.Spec.Traffic.RateLimit.Provider, true)

			// Verify consume route has default rate limits
			defaultRateLimit := apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default.Limits
			verifyConsumeRouteRateLimits(proxyApiSubscription, defaultRateLimit)

			// Clean up
			err = k8sClient.Delete(ctx, proxyApplication)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, proxyApiSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should apply consumer rate limits to ConsumeRoute even when provider rate limits are not specified", func() {
			// Create a new API with only consumer rate limits
			consumerOnlyApiBasePath := "/ratelimit/consumer-only/v1"

			By("Creating the API")
			consumerOnlyApi := NewApi(consumerOnlyApiBasePath)
			err := k8sClient.Create(ctx, consumerOnlyApi)
			Expect(err).ToNot(HaveOccurred())

			By("Creating the APIExposure with only consumer rate limits")
			consumerOnlyApiExposure := NewApiExposureWithConsumerOnlyRateLimit(consumerOnlyApiBasePath, zoneName, application.Status.ClientId, apiExpAppName)
			err = k8sClient.Create(ctx, consumerOnlyApiExposure)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(consumerOnlyApiExposure), consumerOnlyApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumerOnlyApiExposure.Status.Active).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("Creating and approving a subscription")
			consumerOnlySubscription := createAndApproveSubscription(consumerOnlyApiBasePath, zoneName, appName, nil)

			By("Verifying the ConsumeRoute has consumer rate limits even without provider rate limits")
			consumerRateLimit, ok := consumerOnlyApiExposure.GetOverriddenSubscriberRateLimit(application.Status.ClientId)
			Expect(ok).To(BeTrue(), "ApiExposure should have a rate limit override for this consumer")
			verifyConsumeRouteRateLimits(consumerOnlySubscription, consumerRateLimit)

			By("Verifying the Route has no rate limits")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(consumerOnlyApiExposure), consumerOnlyApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumerOnlyApiExposure.Status.Route).ToNot(BeNil())

				// Get the Route
				routeObj := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, consumerOnlyApiExposure.Status.Route.K8s(), routeObj)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify route has no rate limit configuration
				g.Expect(routeObj.Spec.Traffic.RateLimit).To(BeNil(), "Route should not have rate limit configuration")
			}, timeout, interval).Should(Succeed())

			// Clean up
			err = k8sClient.Delete(ctx, consumerOnlySubscription)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, consumerOnlyApiExposure)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, consumerOnlyApi)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not apply consumer rate limits to ConsumeRoute when no consumer rate limits are specified", func() {
			// Create a new API with only provider rate limits
			providerOnlyApiBasePath := "/ratelimit/provider-only/v1"

			By("Creating the API")
			providerOnlyApi := NewApi(providerOnlyApiBasePath)
			err := k8sClient.Create(ctx, providerOnlyApi)
			Expect(err).ToNot(HaveOccurred())

			By("Creating the APIExposure with only provider rate limits")
			providerOnlyApiExposure := NewApiExposureWithProviderOnlyRateLimit(providerOnlyApiBasePath, zoneName, apiExpAppName)
			err = k8sClient.Create(ctx, providerOnlyApiExposure)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(providerOnlyApiExposure), providerOnlyApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(providerOnlyApiExposure.Status.Active).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			By("Creating and approving a subscription")
			providerOnlySubscription := createAndApproveSubscription(providerOnlyApiBasePath, zoneName, appName, nil)

			By("Verifying the Route has provider rate limits")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(providerOnlyApiExposure), providerOnlyApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(providerOnlyApiExposure.Status.Route).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			verifyRouteRateLimits(providerOnlyApiExposure.Status.Route, providerOnlyApiExposure.Spec.Traffic.RateLimit.Provider, false)

			By("Verifying the ConsumeRoute has no rate limits")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(providerOnlySubscription), providerOnlySubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(providerOnlySubscription.Status.ConsumeRoute).ToNot(BeNil())

				// Get the ConsumeRoute
				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, providerOnlySubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify consume route has no rate limit configuration
				if consumeRoute.Spec.Traffic != nil {
					g.Expect(consumeRoute.Spec.Traffic.RateLimit).To(BeNil(), "ConsumeRoute should not have rate limit configuration")
				}
			}, timeout, interval).Should(Succeed())

			// Clean up
			err = k8sClient.Delete(ctx, providerOnlySubscription)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, providerOnlyApiExposure)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Delete(ctx, providerOnlyApi)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
