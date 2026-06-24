// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/test/testutil"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func CreateZone(name string) *adminapi.Zone {
	zone := &adminapi.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testEnvironment,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminapi.ZoneSpec{
			Visibility: adminapi.ZoneVisibilityWorld,
			Gateway: adminapi.GatewayConfig{
				Admin: adminapi.GatewayAdminConfig{
					Url: "http://gateway-admin.test.local:8001",
				},
				Presets: []adminapi.GatewayConfigPreset{
					{
						Name:    "default",
						Default: true,
						Urls: []adminapi.UrlConfig{
							{
								Hostname: fmt.Sprintf("my-gateway.%s", name),
								Scheme:   "http",
								Port:     8080,
								BasePath: "/",
							},
						},
					},
				},
			},
			IdentityProvider: adminapi.IdentityProviderConfig{
				Url: "http://idp.test.local:8080",
				Admin: adminapi.IdentityProviderAdminConfig{
					Url: ptr.To("http://idp-admin.test.local:8080"),
				},
			},
			Redis: &adminapi.RedisConfig{
				Host: "redis://redis.test.local:6379",
			},
		},
	}

	err := k8sClient.Create(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	zone.Status.Namespace = testEnvironment + "--" + name
	zone.Status.Gateway = &types.ObjectRef{
		Name:      "test-gateway",
		Namespace: testEnvironment + "--" + name,
	}
	zone.Status.Links = adminapi.Links{
		Url:       fmt.Sprintf("http://test.%s.de", name),
		Issuer:    fmt.Sprintf("http://issuer.%s.de:8080/auth/realms/test", name),
		LmsIssuer: fmt.Sprintf("http://lms-issuer.%s.de:8080/auth/realms/test", name),
	}
	zone.SetCondition(condition.NewReadyCondition("Ready", "testing"))

	err = k8sClient.Status().Update(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	CreateNamespace(zone.Status.Namespace)
	return zone
}

func NewApiExposure(apiBasePath, zoneName, appName string) *apiv1.ApiExposure {
	return &apiv1.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiBasePath),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiv1.BasePathLabelKey:     labelutil.NormalizeLabelValue(apiBasePath),
				util.ApplicationLabelKey:   labelutil.NormalizeLabelValue(appName),
			},
		},
		Spec: apiv1.ApiExposureSpec{
			ApiBasePath: apiBasePath,
			Upstreams: []apiv1.Upstream{
				{
					Url:    "http://my-provider-api:8080/api/v1",
					Weight: 100,
				},
			},
			Transformation: &apiv1.Transformation{
				Request: apiv1.RequestResponseTransformation{
					Headers: apiv1.HeaderTransformation{
						Remove: []string{"X-Remove-Header"},
					},
				},
			},
			Traffic: apiv1.Traffic{
				RateLimit: &apiv1.RateLimit{
					Provider: &apiv1.RateLimitConfig{
						Limits: apiv1.Limits{
							Second: 100,
							Minute: 1000,
							Hour:   10000,
						},
						Options: apiv1.RateLimitOptions{
							HideClientHeaders: true,
							FaultTolerant:     true,
						},
					},
					SubscriberRateLimit: &apiv1.SubscriberRateLimits{
						Default: &apiv1.SubscriberRateLimitDefaults{
							Limits: apiv1.Limits{
								Second: 10,
								Minute: 100,
								Hour:   1000,
							},
						},
						Overrides: []apiv1.RateLimitOverrides{
							{
								Subscriber: "test-subscriber",
								Limits: apiv1.Limits{
									Second: 10,
									Minute: 100,
									Hour:   1000,
								},
							},
						},
					},
				},
			},
			Security: &apiv1.Security{
				M2M: &apiv1.Machine2MachineAuthentication{
					ExternalIDP: &apiv1.ExternalIdentityProvider{
						TokenEndpoint: "https://example.com/token",
						TokenRequest:  apiv1.TokenRequestClientSecretBasic,
						GrantType:     "client_credentials",
						Client: &apiv1.OAuth2ClientCredentials{
							ClientId:  "client-id",
							ClientKey: "******",
						},
					},

					Scopes: []string{"scope1"},
				},
			},
			Visibility: apiv1.VisibilityWorld,
			Approval:   apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
		Status: apiv1.ApiExposureStatus{},
	}
}

var _ = Describe("ApiExposure Controller", Ordered, func() {
	apiBasePath := "/apiexpctrl/test/v1"
	zoneName := "apiexp-test"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api

	appName := "api-exposure-app-2"
	var apiExpApplication *applicationapi.Application

	BeforeAll(func() {
		By("Creating the Application for ApiExposure")
		apiExpApplication = CreateApplication(appName)

		By("Initializing the API and APIExposure")
		api = NewApi(apiBasePath)
		apiExposure = NewApiExposure(apiBasePath, zoneName, appName)

		By("Creating the Zone")
		CreateZone(zoneName)
	})

	AfterAll(func() {
		By("Deleting the Application for ApiExposure")
		err := k8sClient.Delete(ctx, apiExpApplication)
		Expect(err).ToNot(HaveOccurred())

		By("Cleaning up and deleting all resources")
		err = k8sClient.Delete(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Creating and Updating ApiExposures", Ordered, func() {
		It("should block until an API is registered", func() {
			By("Creating the resource")
			err := k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Active).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})

		It("should automatically progress when an API is registered", func() {
			By("Creating the API resource")
			err := k8sClient.Create(ctx, api)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Active).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should create the real-route", func() {
			By("Checking if the real-route has been created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(route.Spec.Transformation.Request.Headers.Remove).To(ContainElement("X-Remove-Header"))
			}, timeout, interval).Should(Succeed())
		})

		It("should pass rate limit configuration from ApiExposure to Route", func() {
			By("Checking if the rate limit configuration is passed to the route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				g.Expect(apiExposure.HasRateLimit()).To(BeTrue(), "ApiExposure should have rate limit configuration")
				g.Expect(apiExposure.HasProviderRateLimit()).To(BeTrue(), "ApiExposure should have provider rate limit")

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that the route has rate limit configuration
				g.Expect(route.Spec.Traffic.RateLimit).ToNot(BeNil(), "Route should have rate limit in traffic spec")

				// Verify that the provider rate limit values are correctly passed
				g.Expect(route.Spec.Traffic.RateLimit.Limits.Second).To(Equal(apiExposure.Spec.Traffic.RateLimit.Provider.Limits.Second))
				g.Expect(route.Spec.Traffic.RateLimit.Limits.Minute).To(Equal(apiExposure.Spec.Traffic.RateLimit.Provider.Limits.Minute))
				g.Expect(route.Spec.Traffic.RateLimit.Limits.Hour).To(Equal(apiExposure.Spec.Traffic.RateLimit.Provider.Limits.Hour))

				// Verify that the provider rate limit options are correctly passed
				g.Expect(route.Spec.Traffic.RateLimit.Options.HideClientHeaders).To(Equal(apiExposure.Spec.Traffic.RateLimit.Provider.Options.HideClientHeaders))
				g.Expect(route.Spec.Traffic.RateLimit.Options.FaultTolerant).To(Equal(apiExposure.Spec.Traffic.RateLimit.Provider.Options.FaultTolerant))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Deleting and Switching the Active ApiExposure", Ordered, func() {
		var secondApiExposure *apiv1.ApiExposure
		appName := "api-exposure-app-3"
		var apiExpApplication *applicationapi.Application

		BeforeAll(func() {
			By("Creating the Application for ApiExposure")
			apiExpApplication = CreateApplication(appName)

			By("Creating the second APIExposure")
			secondApiExposure = NewApiExposure(apiBasePath, zoneName, appName)
			secondApiExposure.Name = "second-api"
		})

		AfterAll(func() {
			By("Deleting the Application for ApiExposure")
			err := k8sClient.Delete(ctx, apiExpApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should successfully provision the resource and set it to inactive", func() {
			By("Creating the second APIExposure resource")
			err := k8sClient.Create(ctx, secondApiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiExposure), secondApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApiExposure.Status.Active).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})

		It("should switch to the next Exposure when the current one is deleted", func() {
			By("Deleting the first APIExposure")
			err := k8sClient.Delete(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiExposure), secondApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApiExposure.Status.Active).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should remove the Route when the last APIExposure is deleted", func() {
			By("Deleting the second APIExposure")
			err := k8sClient.Delete(ctx, secondApiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiExposure), secondApiExposure)
				g.Expect(err).To(HaveOccurred())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("ApiExposure with ExternalIDPConfig Configured", Ordered, func() {
		var thirdApiExposure *apiv1.ApiExposure
		appName := "api-exposure-app-4"
		var apiExpApplication *applicationapi.Application

		BeforeAll(func() {
			By("Creating the Application for ApiExposure")
			apiExpApplication = CreateApplication(appName)

			By("Creating the second APIExposure")
			thirdApiExposure = NewApiExposure(apiBasePath, zoneName, appName)
			thirdApiExposure.Name = "third-api"
			thirdApiExposure.Spec.Security = &apiv1.Security{
				M2M: &apiv1.Machine2MachineAuthentication{
					ExternalIDP: &apiv1.ExternalIdentityProvider{
						TokenEndpoint: "https://example.com/token",
						Client: &apiv1.OAuth2ClientCredentials{
							ClientId:     "client-id",
							ClientSecret: "******",
						},
					},
				},
			}
		})

		AfterAll(func() {
			By("Deleting the Application for ApiExposure")
			err := k8sClient.Delete(ctx, apiExpApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject invalid config", func() {
			By("Creating the second APIExposure resource")
			thirdApiExposure.Spec.Security.M2M = &apiv1.Machine2MachineAuthentication{
				ExternalIDP: &apiv1.ExternalIdentityProvider{
					TokenRequest: apiv1.TokenRequestMethod("sky"),
				},
				Scopes: []string{"team:scope", "api:scope"},
			}
			err := k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"sky\": supported values: \"client_secret_basic\", \"client_secret_post\""))

			thirdApiExposure.Spec.Security.M2M.ExternalIDP.GrantType = "not_a_valid_grant_type"
			err = k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"not_a_valid_grant_type\": supported values: \"client_credentials\", \"authorization_code\", \"password\""))
		})

		It("should successfully provision the resource", func() {
			By("Creating the second APIExposure resource")
			thirdApiExposure.Spec.Security.M2M = &apiv1.Machine2MachineAuthentication{
				ExternalIDP: &apiv1.ExternalIdentityProvider{
					TokenEndpoint: "https://example.com/token",
					TokenRequest:  apiv1.TokenRequestClientSecretBasic,
					GrantType:     "client_credentials",
					Client: &apiv1.OAuth2ClientCredentials{
						ClientId:     "team",
						ClientSecret: "******",
					},
				},
				Scopes: []string{"team:scope", "api:scope"},
			}

			err := k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(thirdApiExposure), thirdApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("team"))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("******"))
				g.Expect(route.Spec.Security.M2M.Scopes).To(Equal([]string{"team:scope", "api:scope"}))

				g.Expect(route.Spec.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://example.com/token"))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.TokenRequest).To(Equal(gatewayapi.TokenRequestClientSecretBasic))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Validating the basePath", Ordered, func() {
		apiBasePath := "/ApiExpctrl/Test/v1"
		appName := "api-exposure-app-5"
		var apiExpApplication *applicationapi.Application

		var secondApiExposure *apiv1.ApiExposure

		BeforeAll(func() {
			By("Creating the Application for ApiExposure")
			apiExpApplication = CreateApplication(appName)

			secondApiExposure = NewApiExposure(apiBasePath, zoneName, appName)
			secondApiExposure.Name = "case-conflict-api"
		})

		AfterAll(func() {
			By("Deleting the Application for ApiExposure")
			err := k8sClient.Delete(ctx, apiExpApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject basePath with different case", func() {
			By("Creating the second APIExposure resource")
			err := k8sClient.Create(ctx, secondApiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the resource has the expected state")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secondApiExposure), secondApiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(secondApiExposure.Status.Active).To(BeFalse())
				readyCond := meta.FindStatusCondition(secondApiExposure.GetConditions(), condition.ConditionTypeReady)
				testutil.ExpectConditionToBeFalse(g, readyCond, "ApiCaseConflict")
				Expect(readyCond.Message).To(ContainSubstring(`API is registered but the case does not match (got="/ApiExpctrl/Test/v1", found="/apiexpctrl/test/v1").`))
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("ApiExposure Controller with failover scenario", Ordered, func() {
	apiBasePath := "/apiexpctrl/failovertest/v1"
	zoneName := "apiexp-failovertest"
	failoverZoneName := "failover-zone"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api
	var providerZone *adminapi.Zone
	var failoverZone *adminapi.Zone

	BeforeAll(func() {
		By("Creating the Zone")
		providerZone = CreateZone(zoneName)
		By("Creating the Failover Zone")
		failoverZone = CreateZone(failoverZoneName)

		By("Creating the Gateway Client")
		// We need this gateway client because the failover-route is also a proxy routes (in non-failover scenarios)
		// And a proxy-route needs the gateway client for meshing
		CreateGatewayClient(providerZone)

		By("Initializing the API and APIExposure")
		api = NewApi(apiBasePath)
		err := k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		By("Cleaning up and deleting all resources")
		err := k8sClient.Delete(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func(g Gomega) {
			getErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(getErr).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue(), "ApiExposure should be deleted")
		}, timeout, interval).Should(Succeed())

		By("Cleaning up all the created Routes")
		route := &gatewayapi.Route{}
		route.Name = apiExposure.Status.Route.Name
		route.Namespace = apiExposure.Status.Route.Namespace
		err = k8sClient.Delete(ctx, route)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).ToNot(HaveOccurred(), "Failed to delete the route %s/%s", route.Namespace, route.Name)
		}
		route.Name = apiExposure.Status.FailoverRoute.Name
		route.Namespace = apiExposure.Status.FailoverRoute.Namespace
		err = k8sClient.Delete(ctx, route)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).ToNot(HaveOccurred(), "Failed to delete the failover route %s/%s", route.Namespace, route.Name)
		}
	})

	Context("Creating and Updating ApiExposures", Ordered, func() {
		appName := "api-exposure-app-6"
		var apiExpApplication *applicationapi.Application

		BeforeAll(func() {
			By("Creating the Application for ApiExposure")
			apiExpApplication = CreateApplication(appName)
		})

		AfterEach(func() {
			By("Deleting the Application for ApiExposure")
			err := k8sClient.Delete(ctx, apiExpApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create the real- and failover-route", func() {
			By("Creating the resource")
			apiExposure = NewApiExposure(apiBasePath, zoneName, appName)
			apiExposure.Spec.Traffic = apiv1.Traffic{
				Failover: &apiv1.ProviderFailover{
					Zones: []types.ObjectRef{
						{
							Name:      failoverZone.Name,
							Namespace: failoverZone.Namespace,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				testutil.ExpectConditionToBeTrue(g, meta.FindStatusCondition(apiExposure.GetConditions(), condition.ConditionTypeReady), "Provisioned")

				g.Expect(apiExposure.Status.Active).To(BeTrue())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				g.Expect(apiExposure.Status.FailoverRoute).ToNot(BeNil())

				realRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), realRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(realRoute.Spec.Hostnames).To(ContainElement("my-gateway.apiexp-failovertest"))
				Expect(realRoute.Spec.Paths).To(ContainElement("/apiexpctrl/failovertest/v1"))

				Expect(realRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-provider-api"))
				Expect(realRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(realRoute.Spec.Backend.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(realRoute.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))
				Expect(realRoute.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("client-id"))
				Expect(realRoute.Spec.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("******"))

				Expect(realRoute.Spec.Traffic.Failover).To(BeNil())

				failoverRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.FailoverRoute.K8s(), failoverRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(failoverRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-gateway.apiexp-failovertest"))
				Expect(failoverRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(failoverRoute.Spec.Backend.Upstreams[0].Path).To(Equal("/apiexpctrl/failovertest/v1"))

				Expect(failoverRoute.Spec.Traffic.Failover).ToNot(BeNil())

				// The failover secondary route is a proxy-target for cross-zone requests,
				// so the gateway mesh-client must be allowed in its ACL.
				Expect(failoverRoute.Spec.Security).ToNot(BeZero(), "Failover route should have security configured")
				Expect(failoverRoute.Spec.Security.DefaultConsumers).To(ContainElement(util.GatewayConsumerName),
					"Failover secondary route should contain gateway consumer for cross-zone proxy access")

				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Hostname).To(Equal("my-provider-api"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Scheme).To(Equal("http"))
				Expect(failoverRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("Creating and Updating ApiExposures with Security", Ordered, func() {
		secondAppName := "api-exposure-app-7"
		var secondApiExpApplication *applicationapi.Application

		BeforeAll(func() {
			By("Creating the Application for ApiExposure")
			secondApiExpApplication = CreateApplication(secondAppName)
		})

		AfterEach(func() {
			By("Deleting the Application for ApiExposure")
			err := k8sClient.Delete(ctx, secondApiExpApplication)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create the real- and failover-route", func() {
			By("Creating the resource")
			apiExposure = NewApiExposure(apiBasePath, zoneName, secondAppName)
			apiExposure.Spec.Traffic = apiv1.Traffic{
				Failover: &apiv1.ProviderFailover{
					Zones: []types.ObjectRef{
						{
							Name:      failoverZone.Name,
							Namespace: failoverZone.Namespace,
						},
					},
				},
			}
			apiExposure.Spec.Security = &apiv1.Security{
				M2M: &apiv1.Machine2MachineAuthentication{
					ExternalIDP: &apiv1.ExternalIdentityProvider{
						TokenEndpoint: "https://my-issser.example.com/token",
						GrantType:     "password",
						Basic: &apiv1.BasicAuthCredentials{
							Username: "my-username",
							Password: "my-password",
						},
					},
				},
			}

			err := k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Active).To(BeTrue())

				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				g.Expect(apiExposure.Status.FailoverRoute).ToNot(BeNil())

				realRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), realRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(realRoute.Spec.Hostnames).To(ContainElement("my-gateway.apiexp-failovertest"))
				Expect(realRoute.Spec.Paths).To(ContainElement("/apiexpctrl/failovertest/v1"))

				Expect(realRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-provider-api"))
				Expect(realRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(realRoute.Spec.Backend.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(realRoute.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))

				Expect(realRoute.Spec.Traffic.Failover).To(BeNil())

				failoverRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.FailoverRoute.K8s(), failoverRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(failoverRoute.Spec.Backend.Upstreams[0].Hostname).To(Equal("my-gateway.apiexp-failovertest"))
				Expect(failoverRoute.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(failoverRoute.Spec.Backend.Upstreams[0].Path).To(Equal("/apiexpctrl/failovertest/v1"))

				Expect(failoverRoute.Spec.Traffic.Failover).ToNot(BeNil())

				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Hostname).To(Equal("my-provider-api"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Port).To(Equal(int32(8080)))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Scheme).To(Equal("http"))
				Expect(failoverRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))

				Expect(failoverRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://my-issser.example.com/token"))
				Expect(failoverRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.GrantType).To(Equal("password"))
				Expect(failoverRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.Basic.Username).To(Equal("my-username"))
				Expect(failoverRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.Basic.Password).To(Equal("my-password"))
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("ApiExposure Controller cross-zone proxy target ACL", Ordered, func() {
	apiBasePath := "/apiexpctrl/crosszone/v1"
	zoneName := "crosszone-exp-zone"
	otherZoneName := "crosszone-sub-zone"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api

	appName := "crosszone-app"

	var crossZoneSub *apiv1.ApiSubscription

	BeforeAll(func() {
		By("Creating the Application")
		CreateApplication(appName)

		By("Creating the zones")
		zone := CreateZone(zoneName)
		CreateZone(otherZoneName)

		_ = zone // zone is set up with presets in CreateZone

		By("Creating the API")
		api = NewApi(apiBasePath)
		err := k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		err := k8sClient.Delete(ctx, api)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Gateway consumer management based on cross-zone subscriptions", Ordered, func() {
		It("should NOT have gateway consumer in DefaultConsumers when no cross-zone subscriptions exist", func() {
			By("Creating the ApiExposure")
			apiExposure = NewApiExposure(apiBasePath, zoneName, appName)
			err := k8sClient.Create(ctx, apiExposure)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for the real route to be created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Active).To(BeTrue())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				By("Checking that DefaultConsumers does not contain gateway")
				g.Expect(route.Spec.Security.DefaultConsumers).ToNot(ContainElement(util.GatewayConsumerName))
			}, timeout, interval).Should(Succeed())
		})

		It("should add gateway consumer to DefaultConsumers when a cross-zone subscription is created", func() {
			By("Creating a subscription in a different zone")
			crossZoneSub = NewApiSubscription(apiBasePath, otherZoneName, appName)
			err := k8sClient.Create(ctx, crossZoneSub)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for the ApiExposure to re-reconcile and add gateway consumer to the real route")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(apiExposure.Status.Route).ToNot(BeNil())

				route := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(route.Spec.Security).ToNot(BeNil(), "Route security should not be nil when cross-zone subscription exists")
				g.Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(util.GatewayConsumerName),
					"Real route should contain gateway consumer when cross-zone subscription exists")
			}, timeout, interval).Should(Succeed())
		})

		It("should remove gateway consumer from DefaultConsumers when the cross-zone subscription is deleted", func() {
			By("Deleting the cross-zone subscription")
			err := k8sClient.Delete(ctx, crossZoneSub)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for the ApiExposure to re-reconcile and remove gateway consumer from the real route")
			Eventually(func(g Gomega) {
				route := &gatewayapi.Route{}
				err := k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), route)
				g.Expect(err).ToNot(HaveOccurred())

				if route.Spec.Security.DefaultConsumers != nil {
					g.Expect(route.Spec.Security.DefaultConsumers).ToNot(ContainElement(util.GatewayConsumerName),
						"Real route should NOT contain gateway consumer after cross-zone subscription is deleted")
				}
			}, timeout, interval).Should(Succeed())
		})
	})
})
