// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewApiExposureWithConsumerRateLimit creates an ApiExposure with a specific rate limit for a consumer
func NewApiExposureWithConsumerRateLimit(apiBasePath, zoneName, consumerClientId string) *apiapi.ApiExposure {
	return &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiBasePath),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiapi.BasePathLabelKey:    labelutil.NormalizeValue(apiBasePath),
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
							HideClientHeaders: pntBool(false),
							FaultTolerant:     pntBool(true),
						},
					},
					SubscriberRateLimit: &apiapi.SubscriberRateLimits{
						Default: &apiapi.RateLimitConfig{
							Limits: apiapi.Limits{
								Second: 10,
								Minute: 100,
								Hour:   1000,
							},
							Options: apiapi.RateLimitOptions{
								HideClientHeaders: pntBool(true),
								FaultTolerant:     pntBool(true),
							},
						},
						Overrides: []apiapi.RateLimitOverrides{
							{
								Subscriber: consumerClientId,
								Config: apiapi.RateLimitConfig{
									Limits: apiapi.Limits{
										Second: 20,
										Minute: 200,
										Hour:   2000,
									},
									Options: apiapi.RateLimitOptions{
										HideClientHeaders: pntBool(false),
										FaultTolerant:     pntBool(false),
									},
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
			Approval:   apiapi.ApprovalStrategyAuto,
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
	}
}

var _ = Describe("ApiSubscription Rate Limiting", Ordered, func() {
	// API that is used for the tests
	var apiBasePath = "/ratelimit/test/v1"

	// Provider side
	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure

	// Provider/Exposure zone
	var zoneName = "ratelimit-test"
	var zone *adminapi.Zone

	// Consumer side
	var appName = "rate-limit-app"
	var application *applicationapi.Application
	var apiSubscription *apiapi.ApiSubscription

	// Second consumer for default rate limit test
	var defaultAppName = "default-rate-limit-app"
	var defaultApplication *applicationapi.Application
	var defaultApiSubscription *apiapi.ApiSubscription

	BeforeAll(func() {
		By("Creating the Zone")
		zone = CreateZone(zoneName)
		CreateGatewayClient(zone)

		By("Creating the Realm")
		realm := NewRealm(testEnvironment, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the Applications")
		application = CreateApplication(appName)
		defaultApplication = CreateApplication(defaultAppName)
		defaultApplication.Status.ClientId = "default-test-client-id"
		err = k8sClient.Status().Update(ctx, defaultApplication)
		Expect(err).ToNot(HaveOccurred())

		By("Initializing the API")
		api = NewApi(apiBasePath)
		err = k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the APIExposure with consumer-specific rate limit")
		apiExposure = NewApiExposureWithConsumerRateLimit(apiBasePath, zoneName, application.Status.ClientId)
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		// Set APIExposure as active with a route
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())
			apiExposure.Status.Active = true
			apiExposure.Status.Route = &types.ObjectRef{
				Name:      "test-route",
				Namespace: zone.Status.Namespace,
			}
			err = k8sClient.Status().Update(ctx, apiExposure)
			g.Expect(err).ToNot(HaveOccurred())
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		By("Cleaning up and deleting all resources")
		err := k8sClient.Delete(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, application)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Delete(ctx, defaultApplication)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Consumer-specific rate limiting", func() {
		It("should pass consumer-specific rate limit configuration to ConsumeRoute", func() {
			By("Creating the ApiSubscription")
			apiSubscription = NewApiSubscription(apiBasePath, zoneName, appName)
			err := k8sClient.Create(ctx, apiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiSubscription is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalRequest := &approvalapi.ApprovalRequest{}
			err = k8sClient.Get(ctx, apiSubscription.Status.ApprovalRequest.K8s(), approvalRequest)
			Expect(err).ToNot(HaveOccurred())

			// Progress the approval workflow
			approvalRequest = ProgressApprovalRequest(apiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(apiSubscription, approvalapi.ApprovalStateGranted, approvalRequest)

			By("Verifying the ConsumeRoute is created with the consumer-specific rate limit")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiSubscription), apiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiSubscription.Status.ConsumeRoute).ToNot(BeNil())

				// Get the ConsumeRoute
				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, apiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the ConsumeRoute has rate limit configuration
				g.Expect(consumeRoute.Spec.Traffic).ToNot(BeNil(), "ConsumeRoute should have traffic configuration")
				g.Expect(consumeRoute.Spec.Traffic.RateLimit).ToNot(BeNil(), "ConsumeRoute should have rate limit configuration")

				// Get the consumer-specific override from the ApiExposure
				var consumerRateLimit apiapi.RateLimitConfig
				var ok bool
				consumerRateLimit, ok = apiExposure.GetOverriddenSubscriberRateLimit(application.Status.ClientId)
				g.Expect(ok).To(BeTrue(), "ApiExposure should have a rate limit override for this consumer")
				g.Expect(consumerRateLimit).ToNot(BeNil())

				// Verify that the consumer-specific rate limit values are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).To(Equal(consumerRateLimit.Limits.Second))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(consumerRateLimit.Limits.Minute))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(consumerRateLimit.Limits.Hour))

				// Verify that the consumer-specific rate limit options are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.HideClientHeaders).To(Equal(consumerRateLimit.Options.HideClientHeaders))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.FaultTolerant).To(Equal(consumerRateLimit.Options.FaultTolerant))

				// Verify that the consumer-specific rate limit is different from the default
				defaultRateLimit := apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).ToNot(Equal(defaultRateLimit.Limits.Second))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.FaultTolerant).ToNot(Equal(defaultRateLimit.Options.FaultTolerant))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.HideClientHeaders).NotTo(Equal(defaultRateLimit.Options.HideClientHeaders))

			}, timeout, interval).Should(Succeed())
		})

		It("should pass default rate limit configuration to ConsumeRoute when no consumer-specific override exists", func() {
			By("Creating the ApiSubscription for the default application")
			defaultApiSubscription = NewApiSubscription(apiBasePath, zoneName, defaultAppName)
			err := k8sClient.Create(ctx, defaultApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiSubscription is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(defaultApiSubscription), defaultApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(defaultApiSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalRequest := &approvalapi.ApprovalRequest{}
			err = k8sClient.Get(ctx, defaultApiSubscription.Status.ApprovalRequest.K8s(), approvalRequest)
			Expect(err).ToNot(HaveOccurred())

			// Progress the approval workflow
			approvalRequest = ProgressApprovalRequest(defaultApiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(defaultApiSubscription, approvalapi.ApprovalStateGranted, approvalRequest)

			By("Verifying the ConsumeRoute is created with the default rate limit")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(defaultApiSubscription), defaultApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(defaultApiSubscription.Status.ConsumeRoute).ToNot(BeNil())

				// Get the ConsumeRoute
				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, defaultApiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the ConsumeRoute has rate limit configuration
				g.Expect(consumeRoute.Spec.Traffic).ToNot(BeNil(), "ConsumeRoute should have traffic configuration")
				g.Expect(consumeRoute.Spec.Traffic.RateLimit).ToNot(BeNil(), "ConsumeRoute should have rate limit configuration")

				// Verify no consumer-specific override exists for this application
				_, ok := apiExposure.GetOverriddenSubscriberRateLimit(defaultApplication.Status.ClientId)
				g.Expect(ok).To(BeFalse(), "ApiExposure should not have a rate limit override for this consumer")

				// Get the default rate limit from the ApiExposure
				defaultRateLimit := apiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default
				g.Expect(defaultRateLimit).ToNot(BeNil())

				// Verify that the default rate limit values are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).To(Equal(defaultRateLimit.Limits.Second))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(defaultRateLimit.Limits.Minute))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(defaultRateLimit.Limits.Hour))

				// Verify that the default rate limit options are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.HideClientHeaders).To(Equal(defaultRateLimit.Options.HideClientHeaders))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.FaultTolerant).To(Equal(defaultRateLimit.Options.FaultTolerant))

				// Verify that the default rate limit is different from the consumer-specific one
				consumerRateLimit, _ := apiExposure.GetOverriddenSubscriberRateLimit(application.Status.ClientId)
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).ToNot(Equal(consumerRateLimit.Limits.Second))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.HideClientHeaders).ToNot(Equal(consumerRateLimit.Options.HideClientHeaders))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.FaultTolerant).ToNot(Equal(consumerRateLimit.Options.FaultTolerant))

			}, timeout, interval).Should(Succeed())
		})
	})

	Context("No default rate limiting with consumer-specific overrides", func() {
		// API that is used for this specific test
		var noDefaultApiBasePath = "/ratelimit/nodefault/v1"
		var noDefaultApi *apiapi.Api
		var noDefaultApiExposure *apiapi.ApiExposure

		// Applications for this test
		var limitedAppName = "limited-app"
		var unlimitedAppName = "unlimited-app"
		var limitedApplication *applicationapi.Application
		var unlimitedApplication *applicationapi.Application
		var limitedApiSubscription *apiapi.ApiSubscription
		var unlimitedApiSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating the applications for no-default test")
			limitedApplication = CreateApplication(limitedAppName)
			unlimitedApplication = CreateApplication(unlimitedAppName)
			unlimitedApplication.Status.ClientId = "unlimited-test-client-id"
			err := k8sClient.Status().Update(ctx, unlimitedApplication)
			Expect(err).ToNot(HaveOccurred())

			By("Initializing the API for no-default test")
			noDefaultApi = NewApi(noDefaultApiBasePath)
			err = k8sClient.Create(ctx, noDefaultApi)
			Expect(err).ToNot(HaveOccurred())

			By("Creating the APIExposure with only consumer-specific rate limit")
			noDefaultApiExposure = NewApiExposureWithConsumerRateLimit(noDefaultApiBasePath, zoneName, limitedApplication.Status.ClientId)
			noDefaultApiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default = nil

			err = k8sClient.Create(ctx, noDefaultApiExposure)
			Expect(err).ToNot(HaveOccurred())

			// Set APIExposure as active with a route
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(noDefaultApiExposure), noDefaultApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				noDefaultApiExposure.Status.Active = true
				noDefaultApiExposure.Status.Route = &types.ObjectRef{
					Name:      "no-default-test-route",
					Namespace: zone.Status.Namespace,
				}
				err = k8sClient.Status().Update(ctx, noDefaultApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
			}, timeout, interval).Should(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up no-default test resources")
			err := k8sClient.Delete(ctx, noDefaultApiExposure)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, noDefaultApi)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, limitedApplication)
			Expect(err).ToNot(HaveOccurred())

			err = k8sClient.Delete(ctx, unlimitedApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should apply rate limit to application with specific override", func() {
			By("Creating the ApiSubscription for the limited application")
			limitedApiSubscription = NewApiSubscription(noDefaultApiBasePath, zoneName, limitedAppName)
			err := k8sClient.Create(ctx, limitedApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiSubscription is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(limitedApiSubscription), limitedApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(limitedApiSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalRequest := &approvalapi.ApprovalRequest{}
			err = k8sClient.Get(ctx, limitedApiSubscription.Status.ApprovalRequest.K8s(), approvalRequest)
			Expect(err).ToNot(HaveOccurred())

			// Progress the approval workflow
			approvalRequest = ProgressApprovalRequest(limitedApiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(limitedApiSubscription, approvalapi.ApprovalStateGranted, approvalRequest)

			By("Verifying the ConsumeRoute is created with the consumer-specific rate limit")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(limitedApiSubscription), limitedApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(limitedApiSubscription.Status.ConsumeRoute).ToNot(BeNil())

				// Get the ConsumeRoute
				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, limitedApiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the ConsumeRoute has rate limit configuration
				g.Expect(consumeRoute.Spec.Traffic).ToNot(BeNil(), "ConsumeRoute should have traffic configuration")
				g.Expect(consumeRoute.Spec.Traffic.RateLimit).ToNot(BeNil(), "ConsumeRoute should have rate limit configuration")

				// Get the consumer-specific override from the ApiExposure
				var consumerRateLimit apiapi.RateLimitConfig
				var ok bool
				consumerRateLimit, ok = noDefaultApiExposure.GetOverriddenSubscriberRateLimit(limitedApplication.Status.ClientId)
				g.Expect(ok).To(BeTrue(), "ApiExposure should have a rate limit override for this consumer")

				// Verify that the consumer-specific rate limit values are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Second).To(Equal(consumerRateLimit.Limits.Second))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(consumerRateLimit.Limits.Minute))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(consumerRateLimit.Limits.Hour))

				// Verify that the consumer-specific rate limit options are correctly passed
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.HideClientHeaders).To(Equal(consumerRateLimit.Options.HideClientHeaders))
				g.Expect(consumeRoute.Spec.Traffic.RateLimit.Options.FaultTolerant).To(Equal(consumerRateLimit.Options.FaultTolerant))

				// Verify that there is no default rate limit
				g.Expect(noDefaultApiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default).To(BeNil(), "ApiExposure should not have a default rate limit")
			}, timeout, interval).Should(Succeed())
		})

		It("should not apply any rate limit to application without specific override", func() {
			By("Creating the ApiSubscription for the unlimited application")
			unlimitedApiSubscription = NewApiSubscription(noDefaultApiBasePath, zoneName, unlimitedAppName)
			err := k8sClient.Create(ctx, unlimitedApiSubscription)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying the ApiSubscription is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(unlimitedApiSubscription), unlimitedApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(unlimitedApiSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalRequest := &approvalapi.ApprovalRequest{}
			err = k8sClient.Get(ctx, unlimitedApiSubscription.Status.ApprovalRequest.K8s(), approvalRequest)
			Expect(err).ToNot(HaveOccurred())

			// Progress the approval workflow
			approvalRequest = ProgressApprovalRequest(unlimitedApiSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(unlimitedApiSubscription, approvalapi.ApprovalStateGranted, approvalRequest)

			By("Verifying the ConsumeRoute is created without any rate limit")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(unlimitedApiSubscription), unlimitedApiSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(unlimitedApiSubscription.Status.ConsumeRoute).ToNot(BeNil())

				// Get the ConsumeRoute
				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, unlimitedApiSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the ConsumeRoute has no rate limit configuration
				if consumeRoute.Spec.Traffic != nil {
					g.Expect(consumeRoute.Spec.Traffic.RateLimit).To(BeNil(), "ConsumeRoute should not have rate limit configuration")
				}

				// Verify no consumer-specific override exists for this application
				_, ok := noDefaultApiExposure.GetOverriddenSubscriberRateLimit(unlimitedApplication.Status.ClientId)
				g.Expect(ok).To(BeFalse(), "ApiExposure should not have a rate limit override for this consumer")

				// Verify that there is no default rate limit
				g.Expect(noDefaultApiExposure.Spec.Traffic.RateLimit.SubscriberRateLimit.Default).To(BeNil(), "ApiExposure should not have a default rate limit")
			}, timeout, interval).Should(Succeed())
		})
	})
})

func pntBool(b bool) *bool { return &b }
