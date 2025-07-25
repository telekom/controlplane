// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		},
	}

	err := k8sClient.Create(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	zone.Status.Namespace = testEnvironment + "--" + name
	err = k8sClient.Status().Update(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	CreateNamespace(zone.Status.Namespace)
	return zone
}

func NewRealm(name, zoneName string) *gatewayapi.Realm {
	return &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testEnvironment + "--" + zoneName,
			Labels: map[string]string{
				config.EnvironmentLabelKey:   testEnvironment,
				config.BuildLabelKey("zone"): zoneName,
			},
		},
		Spec: gatewayapi.RealmSpec{
			Url:       fmt.Sprintf("http://my-gateway.%s:8080", zoneName),
			IssuerUrl: fmt.Sprintf("http://my-issuer.%s:8080/auth/realms/%s", zoneName, testEnvironment),
		},
	}
}

func NewApiExposure(apiBasePath, zoneName string) *apiv1.ApiExposure {
	return &apiv1.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      labelutil.NormalizeValue(apiBasePath),
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
				apiv1.BasePathLabelKey:     labelutil.NormalizeValue(apiBasePath),
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
			Visibility: apiv1.VisibilityWorld,
			Approval:   apiv1.Approval{Strategy: apiapi.ApprovalStrategyAuto},
			Zone: types.ObjectRef{
				Name:      zoneName,
				Namespace: testEnvironment,
			},
		},
	}
}

var _ = Describe("ApiExposure Controller", Ordered, func() {
	var apiBasePath = "/apiexpctrl/test/v1"
	var zoneName = "apiexp-test"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api
	var zone *adminapi.Zone

	BeforeAll(func() {
		By("Initializing the API and APIExposure")
		api = NewApi(apiBasePath)
		apiExposure = NewApiExposure(apiBasePath, zoneName)

		By("Creating the Zone")
		zone = CreateZone(zoneName)

		By("Creating the Gateway")
		realm := NewRealm(testEnvironment, zone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up and deleting all resources")
		err := k8sClient.Delete(ctx, api)
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
	})

	Context("Deleting and Switching the Active ApiExposure", Ordered, func() {

		var secondApiExposure *apiv1.ApiExposure

		BeforeAll(func() {
			By("Creating the second APIExposure")
			secondApiExposure = NewApiExposure(apiBasePath, zoneName)
			secondApiExposure.Name = "second-api"

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

		BeforeAll(func() {
			By("Creating the second APIExposure")
			thirdApiExposure = NewApiExposure(apiBasePath, zoneName)
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

		It("should reject invalid config", func() {
			By("Creating the second APIExposure resource")
			thirdApiExposure.Spec.Security.M2M = &apiv1.Machine2MachineAuthentication{
				ExternalIDP: &apiv1.ExternalIdentityProvider{
					TokenRequest: "sky",
				},
				Scopes: []string{"team:scope", "api:scope"},
			}
			err := k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"sky\": supported values: \"body\", \"header\""))

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
					TokenRequest:  "header",
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
				g.Expect(route.Spec.Upstreams).To(HaveLen(1))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("team"))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("******"))
				g.Expect(route.Spec.Security.M2M.Scopes).To(Equal([]string{"team:scope", "api:scope"}))

				g.Expect(route.Spec.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://example.com/token"))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.TokenRequest).To(Equal("header"))
				g.Expect(route.Spec.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			}, timeout, interval).Should(Succeed())
		})

	})

})

var _ = Describe("ApiExposure Controller with failover scenario", Ordered, func() {
	var apiBasePath = "/apiexpctrl/failovertest/v1"
	var zoneName = "apiexp-failovertest"
	var failoverZoneName = "failover-zone"

	var apiExposure *apiv1.ApiExposure
	var api *apiv1.Api
	var providerZone *adminapi.Zone
	var failoverZone *adminapi.Zone

	BeforeAll(func() {
		By("Creating the Zone")
		providerZone = CreateZone(zoneName)
		By("Creating the Failover Zone")
		failoverZone = CreateZone(failoverZoneName)

		By("Creating the Gateway")
		realm := NewRealm(testEnvironment, providerZone.Name)
		err := k8sClient.Create(ctx, realm)
		Expect(err).ToNot(HaveOccurred())

		By("Creating the Gateway Client")
		// We need this gateway client because the failover-route is also a proxy routes (in non-failover scenarios)
		// And a proxy-route needs the gateway client for meshing
		CreateGatewayClient(providerZone)

		By("Creating the Failover Gateway")
		failoverRealm := NewRealm(testEnvironment, failoverZone.Name)
		err = k8sClient.Create(ctx, failoverRealm)
		Expect(err).ToNot(HaveOccurred())

		By("Initializing the API and APIExposure")
		api = NewApi(apiBasePath)
		err = k8sClient.Create(ctx, api)
		Expect(err).ToNot(HaveOccurred())

	})

	AfterEach(func() {
		By("Cleaning up and deleting all resources")
		err := k8sClient.Delete(ctx, apiExposure)
		Expect(err).ToNot(HaveOccurred())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(apiExposure), apiExposure)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "ApiExposure should be deleted")
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

		It("should create the real- and failover-route", func() {
			By("Creating the resource")
			apiExposure = NewApiExposure(apiBasePath, zoneName)
			apiExposure.Spec.Traffic = apiv1.Traffic{
				Failover: &apiv1.Failover{
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
				g.Expect(apiExposure.Status.Active).To(BeTrue())

				g.Expect(apiExposure.Status.Route).ToNot(BeNil())
				g.Expect(apiExposure.Status.FailoverRoute).ToNot(BeNil())

				realRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.Route.K8s(), realRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(realRoute.Spec.Downstreams[0].Url()).To(Equal("https://my-gateway.apiexp-failovertest:8080/apiexpctrl/failovertest/v1"))
				Expect(realRoute.Spec.Downstreams[0].IssuerUrl).To(Equal("http://my-issuer.apiexp-failovertest:8080/auth/realms/test"))

				Expect(realRoute.Spec.Upstreams[0].Host).To(Equal("my-provider-api"))
				Expect(realRoute.Spec.Upstreams[0].Port).To(Equal(8080))
				Expect(realRoute.Spec.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(realRoute.Spec.Upstreams[0].Scheme).To(Equal("http"))
				Expect(realRoute.Spec.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("client-id"))
				Expect(realRoute.Spec.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("******"))

				Expect(realRoute.Spec.Traffic.Failover).To(BeNil())

				failoverRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.FailoverRoute.K8s(), failoverRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(failoverRoute.Spec.Upstreams[0].Url()).To(Equal("http://my-gateway.apiexp-failovertest:8080/apiexpctrl/failovertest/v1"))
				Expect(failoverRoute.Spec.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.apiexp-failovertest:8080/auth/realms/test"))

				Expect(failoverRoute.Spec.Traffic.Failover).ToNot(BeNil())

				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Host).To(Equal("my-provider-api"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Port).To(Equal(8080))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Scheme).To(Equal("http"))
				Expect(failoverRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZone.Name))

			}, timeout, interval).Should(Succeed())

		})
	})

	Context("Creating and Updating ApiExposures with Security", Ordered, func() {

		It("should create the real- and failover-route", func() {
			By("Creating the resource")
			apiExposure = NewApiExposure(apiBasePath, zoneName)
			apiExposure.Spec.Traffic = apiv1.Traffic{
				Failover: &apiv1.Failover{
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

				Expect(realRoute.Spec.Downstreams[0].Url()).To(Equal("https://my-gateway.apiexp-failovertest:8080/apiexpctrl/failovertest/v1"))
				Expect(realRoute.Spec.Downstreams[0].IssuerUrl).To(Equal("http://my-issuer.apiexp-failovertest:8080/auth/realms/test"))

				Expect(realRoute.Spec.Upstreams[0].Host).To(Equal("my-provider-api"))
				Expect(realRoute.Spec.Upstreams[0].Port).To(Equal(8080))
				Expect(realRoute.Spec.Upstreams[0].Path).To(Equal("/api/v1"))
				Expect(realRoute.Spec.Upstreams[0].Scheme).To(Equal("http"))

				Expect(realRoute.Spec.Traffic.Failover).To(BeNil())

				failoverRoute := &gatewayapi.Route{}
				err = k8sClient.Get(ctx, apiExposure.Status.FailoverRoute.K8s(), failoverRoute)
				g.Expect(err).ToNot(HaveOccurred())

				Expect(failoverRoute.Spec.Upstreams[0].Url()).To(Equal("http://my-gateway.apiexp-failovertest:8080/apiexpctrl/failovertest/v1"))
				Expect(failoverRoute.Spec.Upstreams[0].IssuerUrl).To(Equal("http://my-issuer.apiexp-failovertest:8080/auth/realms/test"))

				Expect(failoverRoute.Spec.Traffic.Failover).ToNot(BeNil())

				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Host).To(Equal("my-provider-api"))
				Expect(failoverRoute.Spec.Traffic.Failover.Upstreams[0].Port).To(Equal(8080))
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
