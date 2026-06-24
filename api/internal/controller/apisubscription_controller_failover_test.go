// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSubscription Controller with failover scenario", Ordered, func() {
	// Scenario 1:
	// ApiSubscription is in the same zone as the ApiExposure failover zone
	// Normal-Flow: consumerZone -> providerZone -> providerApi
	// Failover-Flow: consumerZone == providerFailoverZone -> providerApi
	// Scenario 2:
	// ApiSubscription is in a different zone as the ApiExposure failover zone
	// Normal-Flow: consumerZone -> providerZone -> providerApi
	// Failover-Flow: consumerZone -> providerFailoverZone -> providerApi
	// Scenario 3:
	// ApiSubscription with multiple failover zones configured
	// Tests creation of multiple failover routes and consume routes
	// Scenario 4:
	// ApiSubscription in different zone as ApiExposure and ApiExposure failover zones
	// ApiSubscription failover zone is the same as the ApiExposure zone

	apiBasePath := "/apisub/failovertest/v1"

	// Provider side
	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure

	// Provider/Exposure zone
	providerZoneName := "provider-zone"
	var providerZone *adminapi.Zone

	// Failover zone for Provider
	failoverZoneName := "apisub-failover-zone"
	var failoverZone *adminapi.Zone

	// Consumer side
	appName := "failover-test-app"
	var application *applicationapi.Application

	BeforeAll(func() {
		By("Creating the provider zone")
		providerZone = CreateZone(providerZoneName)
		CreateGatewayClient(providerZone)

		By("Creating the failover zone")
		failoverZone = CreateZone(failoverZoneName)
		CreateGatewayClient(failoverZone)

		By("Enabling ConsumerFailover feature on failover zone")
		failoverZone.Spec.Gateway.Presets = append(failoverZone.Spec.Gateway.Presets, adminapi.GatewayConfigPreset{
			Name: "consumer-failover",
			Urls: []adminapi.UrlConfig{{
				Hostname: "failover." + failoverZoneName,
				Scheme:   "http",
				Port:     8080,
				BasePath: "/",
			}},
			Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
		})
		Expect(k8sClient.Update(ctx, failoverZone)).To(Succeed())
		failoverZone.EnableFeature(adminapi.FeatureConsumerFailover)
		Expect(k8sClient.Status().Update(ctx, failoverZone)).To(Succeed())

		By("Creating the Application")
		application = CreateApplication(appName)

		By("Initializing the API")
		api = NewApi(apiBasePath)
		err := k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating APIExposure with failover configuration")
		apiExposure = NewApiExposure(apiBasePath, providerZoneName, appName)
		apiExposure.Spec.Traffic = apiapi.Traffic{
			Failover: &apiapi.ProviderFailover{
				Zones: []types.ObjectRef{
					{
						Name:      failoverZone.Name,
						Namespace: failoverZone.Namespace,
					},
				},
			},
		}
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		By("Checking if APIExposure is created with proper failover configuration")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())
			testutil.ExpectConditionToMatch(g, meta.FindStatusCondition(apiExposure.GetConditions(), condition.ConditionTypeReady), "Provisioned", true)

			g.Expect(apiExposure.Status.Active).To(BeTrue())
			g.Expect(apiExposure.Status.Route).ToNot(BeNil())
			g.Expect(apiExposure.Status.FailoverRoute).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
	})

	Context("Same Zone as ApiExposure Failover Zone", func() {
		var sameZoneSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating ApiSubscription in the failover zone")
			sameZoneSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			sameZoneSubscription.Name = "failover-same-zone-subscription"
			sameZoneSubscription.Spec.Zone = types.ObjectRef{
				Name:      failoverZoneName,
				Namespace: testEnvironment,
			}
			err := k8sClient.Create(ctx, sameZoneSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(sameZoneSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(sameZoneSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should reuse the Proxy-Route created as secondary-route by ApiExposure", func() {
			By("Checking route configuration")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, sameZoneSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify route has proper downstream configuration (hostnames and paths)
				g.Expect(route.Spec.Hostnames).To(ContainElement("my-gateway.apisub-failover-zone"))
				g.Expect(route.Spec.Paths).To(ContainElement("/apisub/failovertest/v1"))

				// Verify route has proper upstream configuration pointing to provider zone
				g.Expect(route.Spec.Backend.Upstreams[0].Url()).To(Equal("http://my-gateway.provider-zone:8080/apisub/failovertest/v1"))

				// Verify route has proper failover configuration pointing to provider API
				g.Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
				g.Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal("http://my-provider-api:8080/api/v1"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the ApiSubscription", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSubscription), sameZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, sameZoneSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*sameZoneSubscription.Status.Route))
				g.Expect(consumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Different Zone than ApiExposure Failover Zone", func() {
		differentZoneName := "different-zone"
		var differentZone *adminapi.Zone
		var differentZoneSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating a different zone")
			differentZone = CreateZone(differentZoneName)
			CreateGatewayClient(differentZone)

			By("Creating ApiSubscription in a different zone")
			differentZoneSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			differentZoneSubscription.Name = "failover-different-zone-subscription"
			differentZoneSubscription.Spec.Zone = types.ObjectRef{
				Name:      differentZoneName,
				Namespace: testEnvironment,
			}
			err := k8sClient.Create(ctx, differentZoneSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(differentZoneSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(differentZoneSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			approval := ProgressApproval(differentZoneSubscription, approvalapi.ApprovalStateGranted, approvalReq)
			Expect(approval).ToNot(BeNil())
		})

		It("should create a proxy route with failover that points to the Api-Provider failover zone", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				By("Checking the conditions")
				testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(differentZoneSubscription.GetConditions(), condition.ConditionTypeReady), "Provisioned")
				g.Expect(differentZoneSubscription.Status.Route).ToNot(BeNil())

				By("Verifying route configuration")
				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, differentZoneSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify route has proper downstream configuration (hostnames and paths)
				g.Expect(route.Spec.Hostnames).To(ContainElement("my-gateway.different-zone"))
				g.Expect(route.Spec.Paths).To(ContainElement("/apisub/failovertest/v1"))

				// Verify route has proper upstream configuration pointing to provider zone
				g.Expect(route.Spec.Backend.Upstreams[0].Url()).To(Equal("http://my-gateway.provider-zone:8080/apisub/failovertest/v1"))

				// Verify route has proper failover configuration pointing to provider failover zone
				g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
				g.Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
				g.Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))
				g.Expect(route.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal("http://my-gateway.apisub-failover-zone:8080/apisub/failovertest/v1"))

				g.Expect(route.Labels[config.BuildLabelKey("failover.zone")]).To(Equal(labelutil.NormalizeValue(failoverZone.Name)))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the ApiSubscription", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(differentZoneSubscription), differentZoneSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(differentZoneSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, differentZoneSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*differentZoneSubscription.Status.Route))
				g.Expect(consumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ApiSubscription with Multiple Failover Zones", func() {
		multiFailoverZoneName1 := "multi-failover-zone1"
		multiFailoverZoneName2 := "multi-failover-zone2"
		var multiFailoverZone1, multiFailoverZone2 *adminapi.Zone
		var multiFailoverSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating multiple failover zones")
			multiFailoverZone1 = CreateZone(multiFailoverZoneName1)
			multiFailoverZone2 = CreateZone(multiFailoverZoneName2)
			CreateGatewayClient(multiFailoverZone1)
			CreateGatewayClient(multiFailoverZone2)

			By("Enabling ConsumerFailover feature on failover zones")
			for _, zone := range []*adminapi.Zone{multiFailoverZone1, multiFailoverZone2} {
				zone.Spec.Gateway.Presets = append(zone.Spec.Gateway.Presets, adminapi.GatewayConfigPreset{
					Name: "consumer-failover",
					Urls: []adminapi.UrlConfig{{
						Hostname: "failover." + zone.Name,
						Scheme:   "http",
						Port:     8080,
						BasePath: "/",
					}},
					Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
				})
				Expect(k8sClient.Update(ctx, zone)).To(Succeed())
				zone.EnableFeature(adminapi.FeatureConsumerFailover)
				Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())
			}

			By("Creating ApiSubscription with multiple failover zones")
			multiFailoverSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			multiFailoverSubscription.Name = "multi-failover-zone-subscription"
			multiFailoverSubscription.Spec.Zone = types.ObjectRef{
				Name:      "different-zone",
				Namespace: testEnvironment,
			}
			// Configure failover enabled
			multiFailoverSubscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.SubscriberFailover{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, multiFailoverSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(multiFailoverSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(multiFailoverSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should create a proxy route for the subscription zone", func() {
			By("Checking route configuration")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify proxy route has proper downstream configuration (hostnames and paths)
				g.Expect(route.Spec.Hostnames).To(ContainElement("my-gateway.different-zone"))
				g.Expect(route.Spec.Paths).To(ContainElement("/apisub/failovertest/v1"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create a consume route for the proxy route", func() {
			By("Checking consume route creation")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(multiFailoverSubscription.Status.ConsumeRoute).ToNot(BeNil())

				consumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, multiFailoverSubscription.Status.ConsumeRoute.K8s(), consumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(consumeRoute.Spec.Route).To(Equal(*multiFailoverSubscription.Status.Route))
			}, timeout, interval).Should(Succeed())
		})

		It("should create failover routes for each configured failover zone", func() {
			By("Checking failover routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that at least two failover routes are created (for our configured zones)
				g.Expect(len(multiFailoverSubscription.Status.FailoverRoutes)).To(BeNumerically(">=", 2))

				// Verify routes exist in the expected failover zones
				var namespaces []string
				for _, ref := range multiFailoverSubscription.Status.FailoverRoutes {
					route := &gatewayapi.Route{}
					err = k8sClient.Get(ctx, ref.K8s(), route)
					g.Expect(err).ToNot(HaveOccurred())
					namespaces = append(namespaces, route.Namespace)
				}
				g.Expect(namespaces).To(ContainElement("test--multi-failover-zone1"))
				g.Expect(namespaces).To(ContainElement("test--multi-failover-zone2"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume routes for each failover route", func() {
			By("Checking failover consume routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(multiFailoverSubscription), multiFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that consume routes are created for each failover route
				g.Expect(len(multiFailoverSubscription.Status.FailoverConsumeRoutes)).To(
					BeNumerically(">=", 2))

				// Verify each consume route references a failover route and has correct consumer
				for _, consumeRef := range multiFailoverSubscription.Status.FailoverConsumeRoutes {
					consumeRoute := &gatewayapi.ConsumeRoute{}
					err = k8sClient.Get(ctx, consumeRef.K8s(), consumeRoute)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(consumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ApiSubscription with Failover Zone same as ApiExposure Zone", func() {
		differentZoneName := "another-different-zone"
		var differentZone *adminapi.Zone
		var subscription *apiapi.ApiSubscription

		sameZoneAppName := "same-sub-failover-zone-exp-zone"
		var sameZoneApplication *applicationapi.Application

		BeforeAll(func() {
			By("Creating a different zone")
			differentZone = CreateZone(differentZoneName)
			CreateGatewayClient(differentZone)

			By("Creating the Application")
			sameZoneApplication = CreateApplication(sameZoneAppName)

			By("Creating ApiSubscription in different zone with provider zone as failover")
			subscription = NewApiSubscription(apiBasePath, differentZoneName, sameZoneAppName)
			// Configure failover enabled
			subscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.SubscriberFailover{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, subscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Checking if approval request is created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			By("Approving the subscription")
			approvalReq := ProgressApprovalRequest(subscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(subscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should NOT create a failover route in the provider zone", func() {
			By("Checking failover routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Failover routes should exist (for zones with the feature) but none should
				// target the provider zone since it doesn't have ConsumerFailover enabled.
				g.Expect(len(subscription.Status.FailoverRoutes)).To(BeNumerically(">=", 1))
				for _, ref := range subscription.Status.FailoverRoutes {
					g.Expect(ref.Namespace).ToNot(Equal(providerZone.Status.Namespace),
						"No failover route should be created in provider zone without ConsumerFailover feature")
				}
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume routes for both proxy and failover routes", func() {
			By("Checking consume routes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Check proxy consume route
				g.Expect(subscription.Status.ConsumeRoute).ToNot(BeNil())
				proxyConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, subscription.Status.ConsumeRoute.K8s(), proxyConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(proxyConsumeRoute.Spec.ConsumerName).To(Equal(sameZoneApplication.Status.ClientId))
				g.Expect(proxyConsumeRoute.Spec.Route).To(Equal(*subscription.Status.Route)) // should be the proxy route

				// Check failover consume routes exist for each failover route
				g.Expect(len(subscription.Status.FailoverConsumeRoutes)).To(Equal(len(subscription.Status.FailoverRoutes)))
				for _, consumeRef := range subscription.Status.FailoverConsumeRoutes {
					failoverConsumeRoute := &gatewayapi.ConsumeRoute{}
					err = k8sClient.Get(ctx, consumeRef.K8s(), failoverConsumeRoute)
					g.Expect(err).ToNot(HaveOccurred())
				}
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Approval Denial and Revocation", func() {
		denialZoneName := "approval-denial-zone"
		var denialZone *adminapi.Zone
		var denialSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating zone for approval denial test")
			denialZone = CreateZone(denialZoneName)
			CreateGatewayClient(denialZone)

			By("Creating ApiSubscription in cross-zone")
			denialSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			denialSubscription.Name = "approval-denial-subscription"
			denialSubscription.Spec.Zone = types.ObjectRef{
				Name:      denialZoneName,
				Namespace: testEnvironment,
			}
			err := k8sClient.Create(ctx, denialSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create proxy route when approved", func() {
			By("Approving the subscription")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(denialSubscription), denialSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(denialSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			approvalReq := ProgressApprovalRequest(denialSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(denialSubscription, approvalapi.ApprovalStateGranted, approvalReq)

			By("Verifying proxy route exists after approval")
			Eventually(func(g Gomega) {
				// Check ApiExposure has proxy route for this zone
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.ProxyRoutes).ToNot(BeEmpty())

				// Find proxy route for denial zone
				found := false
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					route := &gatewayapi.Route{}
					err := k8sClient.Get(ctx, proxyRouteRef.K8s(), route)
					if err == nil && route.Namespace == denialZone.Status.Namespace {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Proxy route should exist for denial zone")
			}, timeout, interval).Should(Succeed())
		})

		It("should keep proxy route when approval is denied but subscription still exists", func() {
			By("Denying the approval")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(denialSubscription), denialSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(denialSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			// Update approval to denied
			approvalReq := ProgressApprovalRequest(denialSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateRejected)
			ProgressApproval(denialSubscription, approvalapi.ApprovalStateRejected, approvalReq)

			By("Verifying subscription status is updated to denied")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(denialSubscription), denialSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Subscription should have notready condition
				readyCondition := meta.FindStatusCondition(denialSubscription.Status.Conditions, condition.ConditionTypeReady)
				g.Expect(readyCondition).ToNot(BeNil())
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			}, timeout, interval).Should(Succeed())

			By("Verifying proxy route still exists (route lifecycle is managed by ApiExposure)")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())

				// Proxy route should still exist because the subscription still exists
				found := false
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					route := &gatewayapi.Route{}
					err := k8sClient.Get(ctx, proxyRouteRef.K8s(), route)
					if err == nil && route.Namespace == denialZone.Status.Namespace {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Proxy route should still exist while subscription exists")
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Subscriber Failover to Provider Failover Zone", func() {
		subFailoverToProviderZoneName := "sub-failover-provider-collision-zone"
		var subFailoverToProviderZone *adminapi.Zone
		var subFailoverToProviderSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating subscription zone (cross-zone from exposure)")
			subFailoverToProviderZone = CreateZone(subFailoverToProviderZoneName)
			CreateGatewayClient(subFailoverToProviderZone)

			By("Creating ApiSubscription with subscriber failover to provider failover zone")
			subFailoverToProviderSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			subFailoverToProviderSubscription.Name = "sub-failover-to-provider-zone-subscription"
			subFailoverToProviderSubscription.Spec.Zone = types.ObjectRef{
				Name:      subFailoverToProviderZoneName,
				Namespace: testEnvironment,
			}
			// Subscriber failover enabled
			subFailoverToProviderSubscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.SubscriberFailover{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, subFailoverToProviderSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Approving the subscription")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subFailoverToProviderSubscription), subFailoverToProviderSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subFailoverToProviderSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			approvalReq := ProgressApprovalRequest(subFailoverToProviderSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(subFailoverToProviderSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should create proxy route in subscription zone", func() {
			By("Verifying main proxy route exists in subscription zone")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subFailoverToProviderSubscription), subFailoverToProviderSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subFailoverToProviderSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, subFailoverToProviderSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify it's in the subscription zone
				g.Expect(route.Namespace).To(Equal(subFailoverToProviderZone.Status.Namespace))
				g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
			}, timeout, interval).Should(Succeed())
		})

		It("should reference provider failover route (not create duplicate)", func() {
			By("Verifying subscriber failover references provider failover route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subFailoverToProviderSubscription), subFailoverToProviderSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Should have at least 1 failover route reference including the provider failover zone
				g.Expect(len(subFailoverToProviderSubscription.Status.FailoverRoutes)).To(BeNumerically(">=", 1))

				// Find the failover route in the provider failover zone
				var found bool
				for _, failoverRouteRef := range subFailoverToProviderSubscription.Status.FailoverRoutes {
					route := &gatewayapi.Route{}
					err = k8sClient.Get(ctx, failoverRouteRef.K8s(), route)
					g.Expect(err).ToNot(HaveOccurred())

					if route.Namespace == failoverZone.Status.Namespace {
						found = true
						// CRITICAL: Verify it's the provider failover route (marked as secondary)
						g.Expect(route.Labels[util.LabelFailoverSecondary]).To(Equal("true"),
							"Should reference provider failover route, not create new proxy")

						// Verify it has failover configuration (provider failover characteristics)
						g.Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
						g.Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))

						// Verify it has gateway consumer in ACL (failover secondary routes need this)
						g.Expect(route.Spec.Security).ToNot(BeNil())
						g.Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(util.GatewayConsumerName))
						break
					}
				}
				g.Expect(found).To(BeTrue(),
					"Should have a failover route in the provider failover zone")
			}, timeout, interval).Should(Succeed())
		})

		It("should NOT create duplicate proxy route in provider failover zone", func() {
			By("Verifying ApiExposure doesn't list subscriber failover zone in ProxyRoutes")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())

				// Count routes in provider failover zone namespace
				routesInFailoverZone := 0
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					if proxyRouteRef.Namespace == failoverZone.Status.Namespace {
						routesInFailoverZone++
					}
				}

				// Should be 0 - subscriber failover to provider failover zone is excluded
				// because provider failover route already exists there
				g.Expect(routesInFailoverZone).To(Equal(0),
					"ApiExposure should not create proxy route in provider failover zone for subscriber failover")
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume route for both main and failover routes", func() {
			By("Verifying consume routes exist")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subFailoverToProviderSubscription), subFailoverToProviderSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Main consume route
				g.Expect(subFailoverToProviderSubscription.Status.ConsumeRoute).ToNot(BeNil())
				mainConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, subFailoverToProviderSubscription.Status.ConsumeRoute.K8s(), mainConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Failover consume route - find the one referencing provider failover zone
				g.Expect(len(subFailoverToProviderSubscription.Status.FailoverConsumeRoutes)).To(BeNumerically(">=", 1))

				// Find consume route that references a failover route in the provider failover zone
				var foundConsumeRoute bool
				for i, consumeRef := range subFailoverToProviderSubscription.Status.FailoverConsumeRoutes {
					failoverConsumeRoute := &gatewayapi.ConsumeRoute{}
					err = k8sClient.Get(ctx, consumeRef.K8s(), failoverConsumeRoute)
					g.Expect(err).ToNot(HaveOccurred())

					// Check if this consume route references the failover route in provider failover zone
					if i < len(subFailoverToProviderSubscription.Status.FailoverRoutes) {
						routeRef := subFailoverToProviderSubscription.Status.FailoverRoutes[i]
						route := &gatewayapi.Route{}
						err = k8sClient.Get(ctx, routeRef.K8s(), route)
						g.Expect(err).ToNot(HaveOccurred())
						if route.Namespace == failoverZone.Status.Namespace {
							g.Expect(failoverConsumeRoute.Spec.Route).To(Equal(routeRef))
							foundConsumeRoute = true
							break
						}
					}
				}
				g.Expect(foundConsumeRoute).To(BeTrue(), "Should have consume route referencing provider failover zone route")
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Same-Zone Subscription with Cross-Zone Failover", func() {
		sameZoneFailoverZoneName := "same-zone-sub-failover-zone"
		var sameZoneFailoverZone *adminapi.Zone
		var sameZoneWithFailoverSubscription *apiapi.ApiSubscription

		BeforeAll(func() {
			By("Creating failover zone for same-zone subscription")
			sameZoneFailoverZone = CreateZone(sameZoneFailoverZoneName)
			CreateGatewayClient(sameZoneFailoverZone)

			By("Enabling ConsumerFailover feature on same-zone failover zone")
			sameZoneFailoverZone.Spec.Gateway.Presets = append(sameZoneFailoverZone.Spec.Gateway.Presets, adminapi.GatewayConfigPreset{
				Name: "consumer-failover",
				Urls: []adminapi.UrlConfig{{
					Hostname: "failover." + sameZoneFailoverZoneName,
					Scheme:   "http",
					Port:     8080,
					BasePath: "/",
				}},
				Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
			})
			Expect(k8sClient.Update(ctx, sameZoneFailoverZone)).To(Succeed())
			sameZoneFailoverZone.EnableFeature(adminapi.FeatureConsumerFailover)
			Expect(k8sClient.Status().Update(ctx, sameZoneFailoverZone)).To(Succeed())

			By("Creating ApiSubscription in same zone as exposure with cross-zone failover")
			sameZoneWithFailoverSubscription = NewApiSubscription(apiBasePath, providerZoneName, appName)
			sameZoneWithFailoverSubscription.Name = "same-zone-with-failover-subscription"
			// CRITICAL: Subscription in same zone as exposure (provider-zone)
			sameZoneWithFailoverSubscription.Spec.Zone = types.ObjectRef{
				Name:      providerZoneName,
				Namespace: testEnvironment,
			}
			// But with failover enabled
			sameZoneWithFailoverSubscription.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.SubscriberFailover{
					Enabled: true,
				},
			}
			err := k8sClient.Create(ctx, sameZoneWithFailoverSubscription)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should be approved when subscription is created", func() {
			By("Approving the subscription")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneWithFailoverSubscription), sameZoneWithFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneWithFailoverSubscription.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())

			approvalReq := ProgressApprovalRequest(sameZoneWithFailoverSubscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(sameZoneWithFailoverSubscription, approvalapi.ApprovalStateGranted, approvalReq)
		})

		It("should reference real route (not proxy) for main route", func() {
			By("Verifying main route references real route in exposure zone")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneWithFailoverSubscription), sameZoneWithFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(sameZoneWithFailoverSubscription.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, sameZoneWithFailoverSubscription.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify it's the REAL route (not proxy) in provider zone
				g.Expect(route.Namespace).To(Equal(providerZone.Status.Namespace))
				g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("real"))

				// Real route should point to actual upstreams (not gateway proxy)
				g.Expect(route.Spec.Backend.Upstreams).ToNot(BeEmpty())
				g.Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-provider-api"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create proxy route in failover zone", func() {
			By("Verifying ApiExposure created proxy route in subscriber failover zone")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())

				// Find proxy route in failover zone
				found := false
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					if proxyRouteRef.Namespace == sameZoneFailoverZone.Status.Namespace {
						route := &gatewayapi.Route{}
						err := k8sClient.Get(ctx, proxyRouteRef.K8s(), route)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "ApiExposure should create proxy route in subscriber failover zone")
			}, timeout, interval).Should(Succeed())
		})

		It("should reference proxy route for failover", func() {
			By("Verifying subscription failover references proxy route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneWithFailoverSubscription), sameZoneWithFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Should have at least 1 failover route reference including same-zone failover zone
				g.Expect(len(sameZoneWithFailoverSubscription.Status.FailoverRoutes)).To(BeNumerically(">=", 1))

				// Find failover route in the expected zone
				var found bool
				for _, failoverRouteRef := range sameZoneWithFailoverSubscription.Status.FailoverRoutes {
					route := &gatewayapi.Route{}
					err = k8sClient.Get(ctx, failoverRouteRef.K8s(), route)
					g.Expect(err).ToNot(HaveOccurred())

					if route.Namespace == sameZoneFailoverZone.Status.Namespace {
						found = true
						// Verify it's a proxy route (not real, not provider failover secondary)
						g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
						g.Expect(route.Labels[util.LabelFailoverSecondary]).ToNot(Equal("true"),
							"Should be regular proxy route, not provider failover secondary")

						// Verify it proxies to provider zone
						g.Expect(route.Spec.Backend.Upstreams).ToNot(BeEmpty())
						g.Expect(route.Spec.Backend.Upstreams[0].Hostname).To(ContainSubstring("gateway"))
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Should have failover route in sameZoneFailoverZone")
			}, timeout, interval).Should(Succeed())
		})

		It("should create consume routes for both main and failover", func() {
			By("Verifying consume routes exist")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneWithFailoverSubscription), sameZoneWithFailoverSubscription)
				g.Expect(err).ToNot(HaveOccurred())

				// Main consume route
				g.Expect(sameZoneWithFailoverSubscription.Status.ConsumeRoute).ToNot(BeNil())
				mainConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, sameZoneWithFailoverSubscription.Status.ConsumeRoute.K8s(), mainConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(mainConsumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))

				// Failover consume route - at least 1 exists with correct consumer
				g.Expect(len(sameZoneWithFailoverSubscription.Status.FailoverConsumeRoutes)).To(BeNumerically(">=", 1))
				failoverConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, sameZoneWithFailoverSubscription.Status.FailoverConsumeRoutes[0].K8s(), failoverConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(failoverConsumeRoute.Spec.ConsumerName).To(Equal(application.Status.ClientId))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Multiple Subscriptions Sharing Zone with Different Failover Configs", func() {
		sharedZoneName := "multi-sub-shared-zone"
		var sharedZone *adminapi.Zone
		sharedFailoverZoneName := "multi-sub-failover-zone"
		var sharedFailoverZone *adminapi.Zone
		var subscription1 *apiapi.ApiSubscription // No failover
		var subscription2 *apiapi.ApiSubscription // With failover

		BeforeAll(func() {
			By("Creating shared zone for both subscriptions")
			sharedZone = CreateZone(sharedZoneName)
			CreateGatewayClient(sharedZone)

			By("Creating failover zone for subscription2")
			sharedFailoverZone = CreateZone(sharedFailoverZoneName)
			CreateGatewayClient(sharedFailoverZone)

			By("Enabling ConsumerFailover feature on shared failover zone")
			sharedFailoverZone.Spec.Gateway.Presets = append(sharedFailoverZone.Spec.Gateway.Presets, adminapi.GatewayConfigPreset{
				Name: "consumer-failover",
				Urls: []adminapi.UrlConfig{{
					Hostname: "failover." + sharedFailoverZoneName,
					Scheme:   "http",
					Port:     8080,
					BasePath: "/",
				}},
				Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
			})
			Expect(k8sClient.Update(ctx, sharedFailoverZone)).To(Succeed())
			sharedFailoverZone.EnableFeature(adminapi.FeatureConsumerFailover)
			Expect(k8sClient.Status().Update(ctx, sharedFailoverZone)).To(Succeed())

			By("Creating ApiSubscription 1 (no failover)")
			subscription1 = NewApiSubscription(apiBasePath, providerZoneName, appName)
			subscription1.Name = "multi-sub-no-failover"
			subscription1.Spec.Zone = types.ObjectRef{
				Name:      sharedZoneName,
				Namespace: testEnvironment,
			}
			// No failover configured
			err := k8sClient.Create(ctx, subscription1)
			Expect(err).ToNot(HaveOccurred())

			By("Creating ApiSubscription 2 (with failover)")
			subscription2 = NewApiSubscription(apiBasePath, providerZoneName, appName)
			subscription2.Name = "multi-sub-with-failover"
			subscription2.Spec.Zone = types.ObjectRef{
				Name:      sharedZoneName, // Same zone as subscription1
				Namespace: testEnvironment,
			}
			subscription2.Spec.Traffic = apiapi.SubscriberTraffic{
				Failover: &apiapi.SubscriberFailover{
					Enabled: true,
				},
			}
			err = k8sClient.Create(ctx, subscription2)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should approve both subscriptions", func() {
			By("Approving subscription 1")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription1), subscription1)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription1.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())
			approvalReq1 := ProgressApprovalRequest(subscription1.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(subscription1, approvalapi.ApprovalStateGranted, approvalReq1)

			By("Approving subscription 2")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription2), subscription2)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription2.Status.ApprovalRequest).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())
			approvalReq2 := ProgressApprovalRequest(subscription2.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
			ProgressApproval(subscription2, approvalapi.ApprovalStateGranted, approvalReq2)
		})

		It("should create only ONE proxy route in shared zone (deduplication)", func() {
			By("Verifying single proxy route in shared zone")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())

				// Count proxy routes in shared zone
				routesInSharedZone := 0
				var sharedProxyRouteRef *types.ObjectRef
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					if proxyRouteRef.Namespace == sharedZone.Status.Namespace {
						routesInSharedZone++
						sharedProxyRouteRef = &proxyRouteRef
					}
				}

				// CRITICAL: Should be exactly 1, not 2 (deduplication working)
				g.Expect(routesInSharedZone).To(Equal(1),
					"ApiExposure should create only ONE proxy route in shared zone (deduplication)")

				// Verify the route exists
				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, sharedProxyRouteRef.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
			}, timeout, interval).Should(Succeed())
		})

		It("should create proxy route in failover zone", func() {
			By("Verifying proxy route exists in failover zone")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())

				// Find proxy route in failover zone
				found := false
				for _, proxyRouteRef := range apiExposure.Status.ProxyRoutes {
					if proxyRouteRef.Namespace == sharedFailoverZone.Status.Namespace {
						route := &gatewayapi.Route{}
						err := k8sClient.Get(ctx, proxyRouteRef.K8s(), route)
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "ApiExposure should create proxy route in failover zone")
			}, timeout, interval).Should(Succeed())
		})

		It("should have both subscriptions reference the same proxy route in shared zone", func() {
			By("Verifying both subscriptions reference same route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription1), subscription1)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription1.Status.Route).ToNot(BeNil())

				err = k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription2), subscription2)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(subscription2.Status.Route).ToNot(BeNil())

				// CRITICAL: Both should reference the exact same route (deduplication)
				g.Expect(subscription1.Status.Route.Name).To(Equal(subscription2.Status.Route.Name))
				g.Expect(subscription1.Status.Route.Namespace).To(Equal(subscription2.Status.Route.Namespace))

				// Verify they're both in shared zone
				g.Expect(subscription1.Status.Route.Namespace).To(Equal(sharedZone.Status.Namespace))
			}, timeout, interval).Should(Succeed())
		})

		It("should have subscription1 with no failover routes", func() {
			By("Verifying subscription1 has no failover")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription1), subscription1)
				g.Expect(err).ToNot(HaveOccurred())

				// Subscription1 has no failover configured
				g.Expect(subscription1.Status.FailoverRoutes).To(BeEmpty())
				g.Expect(subscription1.Status.FailoverConsumeRoutes).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("should have subscription2 with failover route", func() {
			By("Verifying subscription2 has failover route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription2), subscription2)
				g.Expect(err).ToNot(HaveOccurred())

				// Subscription2 should have at least 1 failover route including sharedFailoverZone
				g.Expect(len(subscription2.Status.FailoverRoutes)).To(BeNumerically(">=", 1))

				// Find route in the expected failover zone
				var found bool
				for _, routeRef := range subscription2.Status.FailoverRoutes {
					route := &gatewayapi.Route{}
					err = k8sClient.Get(ctx, routeRef.K8s(), route)
					g.Expect(err).ToNot(HaveOccurred())
					if route.Namespace == sharedFailoverZone.Status.Namespace {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "Should have failover route in sharedFailoverZone")

				// Verify failover consume route exists
				g.Expect(len(subscription2.Status.FailoverConsumeRoutes)).To(BeNumerically(">=", 1))
			}, timeout, interval).Should(Succeed())
		})
	})
})

// Test #5: All Subscriptions in Same Zone as Exposure (Isolated)
var _ = Describe("ApiSubscription Controller - All Subscriptions Same Zone", Ordered, func() {
	// ISOLATED TEST: Uses unique apiBasePath to avoid pollution from other tests
	apiBasePath := "/apisub/samezone/v1"

	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure
	providerZoneName := "samezone-provider"
	var providerZone *adminapi.Zone
	appName := "samezone-app"

	var sameZoneSub1 *apiapi.ApiSubscription
	var sameZoneSub2 *apiapi.ApiSubscription
	var sameZoneSub3 *apiapi.ApiSubscription

	BeforeAll(func() {
		By("Creating the provider zone")
		providerZone = CreateZone(providerZoneName)
		CreateGatewayClient(providerZone)

		By("Creating the Application")
		CreateApplication(appName)

		By("Creating the API")
		api = NewApi(apiBasePath)
		err := k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the ApiExposure")
		apiExposure = NewApiExposure(apiBasePath, providerZoneName, appName)
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for ApiExposure to be ready")
		Eventually(func(g Gomega) {
			getErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(getErr).ToNot(HaveOccurred())
			testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(apiExposure.GetConditions(), condition.ConditionTypeReady), "Provisioned")
		}, timeout, interval).Should(Succeed())

		By("Creating multiple subscriptions all in exposure zone")
		sameZoneSub1 = NewApiSubscription(apiBasePath, providerZoneName, appName)
		sameZoneSub1.Name = "samezone-sub1"
		sameZoneSub1.Spec.Zone = types.ObjectRef{
			Name:      providerZoneName,
			Namespace: testEnvironment,
		}
		err = k8sClient.Create(ctx, sameZoneSub1)
		Expect(err).ToNot(HaveOccurred())

		sameZoneSub2 = NewApiSubscription(apiBasePath, providerZoneName, appName)
		sameZoneSub2.Name = "samezone-sub2"
		sameZoneSub2.Spec.Zone = types.ObjectRef{
			Name:      providerZoneName,
			Namespace: testEnvironment,
		}
		err = k8sClient.Create(ctx, sameZoneSub2)
		Expect(err).ToNot(HaveOccurred())

		sameZoneSub3 = NewApiSubscription(apiBasePath, providerZoneName, appName)
		sameZoneSub3.Name = "samezone-sub3"
		sameZoneSub3.Spec.Zone = types.ObjectRef{
			Name:      providerZoneName,
			Namespace: testEnvironment,
		}
		err = k8sClient.Create(ctx, sameZoneSub3)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should approve all subscriptions", func() {
		By("Approving all three subscriptions")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub1), sameZoneSub1)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub1.Status.ApprovalRequest).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
		approvalReq1 := ProgressApprovalRequest(sameZoneSub1.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sameZoneSub1, approvalapi.ApprovalStateGranted, approvalReq1)

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub2), sameZoneSub2)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub2.Status.ApprovalRequest).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
		approvalReq2 := ProgressApprovalRequest(sameZoneSub2.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sameZoneSub2, approvalapi.ApprovalStateGranted, approvalReq2)

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub3), sameZoneSub3)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub3.Status.ApprovalRequest).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
		approvalReq3 := ProgressApprovalRequest(sameZoneSub3.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(sameZoneSub3, approvalapi.ApprovalStateGranted, approvalReq3)
	})

	It("should NOT create any cross-zone proxy routes", func() {
		By("Verifying ApiExposure has no cross-zone proxy routes")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())

			// Since all subscriptions are in the same zone as the exposure,
			// there should be NO proxy routes at all
			g.Expect(apiExposure.Status.ProxyRoutes).To(BeEmpty(),
				"ApiExposure should not create any proxy routes when all subs are in same zone")
		}, timeout, interval).Should(Succeed())
	})

	It("should have all subscriptions reference the real route", func() {
		By("Verifying all three subscriptions reference real route")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub1), sameZoneSub1)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub1.Status.Route).ToNot(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub2), sameZoneSub2)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub2.Status.Route).ToNot(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub3), sameZoneSub3)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub3.Status.Route).ToNot(BeNil())

			// All three should reference the same real route
			g.Expect(sameZoneSub1.Status.Route.Name).To(Equal(sameZoneSub2.Status.Route.Name))
			g.Expect(sameZoneSub2.Status.Route.Name).To(Equal(sameZoneSub3.Status.Route.Name))
			g.Expect(sameZoneSub1.Status.Route.Namespace).To(Equal(providerZone.Status.Namespace))

			// Verify it's the real route
			route := &gatewayapi.Route{}
			err = k8sClient.Get(ctx, sameZoneSub1.Status.Route.K8s(), route)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("real"))
		}, timeout, interval).Should(Succeed())
	})

	It("should have real route point to actual upstreams (not gateway proxy)", func() {
		By("Verifying real route configuration")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub1), sameZoneSub1)
			g.Expect(err).ToNot(HaveOccurred())

			route := &gatewayapi.Route{}
			err = k8sClient.Get(ctx, sameZoneSub1.Status.Route.K8s(), route)
			g.Expect(err).ToNot(HaveOccurred())

			// Real route should point to actual provider API (not gateway proxy)
			g.Expect(route.Spec.Backend.Upstreams).ToNot(BeEmpty())
			g.Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-provider-api"),
				"Real route should point to actual API, not gateway proxy")
		}, timeout, interval).Should(Succeed())
	})

	It("should create separate consume routes for each subscription", func() {
		By("Verifying each subscription has its own consume route")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub1), sameZoneSub1)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub1.Status.ConsumeRoute).ToNot(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub2), sameZoneSub2)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub2.Status.ConsumeRoute).ToNot(BeNil())

			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sameZoneSub3), sameZoneSub3)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(sameZoneSub3.Status.ConsumeRoute).ToNot(BeNil())

			// All consume routes should reference the same route but be different consume routes
			consumeRoute1 := &gatewayapi.ConsumeRoute{}
			err = k8sClient.Get(ctx, sameZoneSub1.Status.ConsumeRoute.K8s(), consumeRoute1)
			g.Expect(err).ToNot(HaveOccurred())

			consumeRoute2 := &gatewayapi.ConsumeRoute{}
			err = k8sClient.Get(ctx, sameZoneSub2.Status.ConsumeRoute.K8s(), consumeRoute2)
			g.Expect(err).ToNot(HaveOccurred())

			consumeRoute3 := &gatewayapi.ConsumeRoute{}
			err = k8sClient.Get(ctx, sameZoneSub3.Status.ConsumeRoute.K8s(), consumeRoute3)
			g.Expect(err).ToNot(HaveOccurred())

			// Different consume routes
			g.Expect(consumeRoute1.Name).ToNot(Equal(consumeRoute2.Name))
			g.Expect(consumeRoute2.Name).ToNot(Equal(consumeRoute3.Name))

			// But all reference the same route
			g.Expect(consumeRoute1.Spec.Route).To(Equal(consumeRoute2.Spec.Route))
			g.Expect(consumeRoute2.Spec.Route).To(Equal(consumeRoute3.Spec.Route))
		}, timeout, interval).Should(Succeed())
	})
})

// Test #6: Provider Failover Zone Reuse with Subscriber Failover (Isolated)
var _ = Describe("ApiSubscription Controller - Provider Failover Reuse", Ordered, func() {
	// ISOLATED TEST: Uses unique apiBasePath to avoid pollution from other tests
	// Scenario: ApiExposure has provider failover zone B
	//           Subscription in zone A with subscriber failover to zone B
	//           Expectation: No duplicate proxy route created, subscriber reuses provider failover route
	apiBasePath := "/apisub/provfailover/v1"

	var api *apiapi.Api
	var apiExposure *apiapi.ApiExposure
	providerZoneName := "provfailover-main"
	var providerZone *adminapi.Zone
	providerFailoverZoneName := "provfailover-secondary"
	var providerFailoverZone *adminapi.Zone
	subscriberZoneName := "provfailover-subscriber"
	var subscriberZone *adminapi.Zone
	appName := "provfailover-app"

	var subscription *apiapi.ApiSubscription

	BeforeAll(func() {
		By("Creating the provider main zone")
		providerZone = CreateZone(providerZoneName)
		CreateGatewayClient(providerZone)

		By("Creating the provider failover zone")
		providerFailoverZone = CreateZone(providerFailoverZoneName)
		CreateGatewayClient(providerFailoverZone)

		By("Enabling ConsumerFailover feature on provider failover zone")
		providerFailoverZone.Spec.Gateway.Presets = append(providerFailoverZone.Spec.Gateway.Presets, adminapi.GatewayConfigPreset{
			Name: "consumer-failover",
			Urls: []adminapi.UrlConfig{{
				Hostname: "failover." + providerFailoverZoneName,
				Scheme:   "http",
				Port:     8080,
				BasePath: "/",
			}},
			Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
		})
		Expect(k8sClient.Update(ctx, providerFailoverZone)).To(Succeed())
		providerFailoverZone.EnableFeature(adminapi.FeatureConsumerFailover)
		Expect(k8sClient.Status().Update(ctx, providerFailoverZone)).To(Succeed())

		By("Creating the subscriber zone")
		subscriberZone = CreateZone(subscriberZoneName)
		CreateGatewayClient(subscriberZone)

		By("Creating the Application")
		CreateApplication(appName)

		By("Creating the API")
		api = NewApi(apiBasePath)
		err := k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the ApiExposure with provider failover")
		apiExposure = NewApiExposure(apiBasePath, providerZoneName, appName)
		apiExposure.Spec.Traffic.Failover = &apiapi.ProviderFailover{
			Zones: []types.ObjectRef{
				{
					Name:      providerFailoverZoneName,
					Namespace: testEnvironment,
				},
			},
		}
		err = k8sClient.Create(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for ApiExposure to be ready")
		Eventually(func(g Gomega) {
			getErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(getErr).ToNot(HaveOccurred())
			testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(apiExposure.GetConditions(), condition.ConditionTypeReady), "Provisioned")
		}, timeout, interval).Should(Succeed())

		By("Creating subscription in different zone with failover to provider failover zone")
		subscription = NewApiSubscription(apiBasePath, subscriberZoneName, appName)
		subscription.Name = "provfailover-sub"
		subscription.Spec.Zone = types.ObjectRef{
			Name:      subscriberZoneName,
			Namespace: testEnvironment,
		}
		subscription.Spec.Traffic.Failover = &apiapi.SubscriberFailover{
			Enabled: true,
		}
		err = k8sClient.Create(ctx, subscription)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should approve the subscription", func() {
		By("Approving the subscription")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(subscription.Status.ApprovalRequest).ToNot(BeNil())
		}, timeout, interval).Should(Succeed())
		approvalReq := ProgressApprovalRequest(subscription.Status.ApprovalRequest, approvalapi.ApprovalStateGranted)
		ProgressApproval(subscription, approvalapi.ApprovalStateGranted, approvalReq)
	})

	It("should have ApiExposure create proxy route for subscriber zone", func() {
		By("Verifying ApiExposure proxy routes")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).ToNot(HaveOccurred())

			// Should have at least 1 proxy route including the subscriber zone
			g.Expect(len(apiExposure.Status.ProxyRoutes)).To(BeNumerically(">=", 1),
				"Should have at least one proxy route")

			// Verify there is a proxy in subscriber zone
			var found bool
			for _, proxyRef := range apiExposure.Status.ProxyRoutes {
				if proxyRef.Namespace == subscriberZone.Status.Namespace {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"Proxy route should be in subscriber zone")
		}, timeout, interval).Should(Succeed())
	})

	It("should have subscription reference subscriber zone proxy for main route", func() {
		By("Verifying subscription main route")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(subscription.Status.Route).ToNot(BeNil())

			// Main route should be the proxy in subscriber zone
			g.Expect(subscription.Status.Route.Namespace).To(Equal(subscriberZone.Status.Namespace))

			route := &gatewayapi.Route{}
			err = k8sClient.Get(ctx, subscription.Status.Route.K8s(), route)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(route.Labels[config.BuildLabelKey("type")]).To(Equal("proxy"))
		}, timeout, interval).Should(Succeed())
	})

	It("should have subscription reuse provider failover route (not create duplicate)", func() {
		By("Verifying subscription failover routes")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
			g.Expect(err).ToNot(HaveOccurred())

			// Should have at least 1 failover route including the provider failover zone
			g.Expect(len(subscription.Status.FailoverRoutes)).To(BeNumerically(">=", 1),
				"Should reference at least one failover route")

			// Find the failover route in the provider failover zone
			var found bool
			for _, ref := range subscription.Status.FailoverRoutes {
				if ref.Namespace == providerFailoverZone.Status.Namespace {
					found = true
					// Verify the route exists and has failover.secondary=true label
					route := &gatewayapi.Route{}
					err = k8sClient.Get(ctx, ref.K8s(), route)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(route.Labels[config.BuildLabelKey("failover.secondary")]).To(Equal("true"),
						"Provider failover route should be marked as secondary failover")
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"Failover route should be in provider failover zone")
		}, timeout, interval).Should(Succeed())
	})

	It("should create failover consume route", func() {
		By("Verifying failover consume route exists")
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(subscription), subscription)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(len(subscription.Status.FailoverConsumeRoutes)).To(BeNumerically(">=", 1))

			// Find a consume route that references the provider failover zone
			var found bool
			for _, consumeRef := range subscription.Status.FailoverConsumeRoutes {
				failoverConsumeRoute := &gatewayapi.ConsumeRoute{}
				err = k8sClient.Get(ctx, consumeRef.K8s(), failoverConsumeRoute)
				g.Expect(err).ToNot(HaveOccurred())
				if failoverConsumeRoute.Spec.Route.Namespace == providerFailoverZone.Status.Namespace {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(),
				"Should have a consume route referencing the provider failover zone")
		}, timeout, interval).Should(Succeed())
	})
})
