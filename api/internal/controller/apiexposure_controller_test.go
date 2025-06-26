// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	adminapi "github.com/telekom/controlplane/admin/api/v1"
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
		Spec: adminapi.ZoneSpec{},
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
			Url:       "http://localhost:8080",
			IssuerUrl: "http://localhost:8080/auth/realms/test",
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
					Url:    "http://localhost:8080",
					Weight: 100,
				},
			},
			Visibility: apiv1.VisibilityWorld,
			Approval:   apiv1.ApprovalStrategyAuto,
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

	Context("ApiExposure with ExternalIDP Configured", Ordered, func() {

		var thirdApiExposure *apiv1.ApiExposure

		BeforeAll(func() {
			By("Creating the second APIExposure")
			thirdApiExposure = NewApiExposure(apiBasePath, zoneName)
			thirdApiExposure.Name = "third-api"
			thirdApiExposure.Spec.TokenEndpoint = "example.com/token"
		})

		It("should reject invalid config", func() {
			By("Creating the second APIExposure resource")
			thirdApiExposure.Spec.Security.Oauth2 = apiv1.Oauth2{
				Scopes:       []string{"team:scope", "api:scope"},
				TokenRequest: "sky",
			}
			err := k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"sky\": supported values: \"body\", \"header\""))

			thirdApiExposure.Spec.Security.Oauth2 = apiv1.Oauth2{
				Scopes:    []string{"team:scope", "api:scope"},
				GrantType: "not_a_valid_grant_type",
			}
			err = k8sClient.Create(ctx, thirdApiExposure)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("Unsupported value: \"not_a_valid_grant_type\": supported values: \"client_credentials\", \"authorization_code\", \"password\""))
		})
		It("should successfully provision the resource", func() {
			By("Creating the second APIExposure resource")
			thirdApiExposure.Spec.Security.Oauth2 = apiv1.Oauth2{
				Scopes:       []string{"team:scope", "api:scope"},
				TokenRequest: "header",
				ClientId:     "team",
				ClientSecret: "******",
				GrantType:    "client_credentials",
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
				checkUpstream(g, route, &gatewayapi.Upstream{
					ClientId:      "team",
					ClientSecret:  "******",
					TokenEndpoint: "example.com/token",
					TokenRequest:  "header",
					GrantType:     "client_credentials",
					Scopes:        []string{"team:scope", "api:scope"},
				})

			}, timeout, interval).Should(Succeed())
		})

	})

})

func checkUpstream(g Gomega, route *gatewayapi.Route, expectedUpstreamObj *gatewayapi.Upstream) {
	g.Expect(route.Spec.Upstreams).To(HaveLen(1))
	g.Expect(route.Spec.Upstreams[0].ClientId).To(Equal(expectedUpstreamObj.ClientId))
	g.Expect(route.Spec.Upstreams[0].ClientSecret).To(Equal(expectedUpstreamObj.ClientSecret))
	g.Expect(route.Spec.Upstreams[0].TokenEndpoint).To(Equal(expectedUpstreamObj.TokenEndpoint))
	g.Expect(route.Spec.Upstreams[0].TokenRequest).To(Equal(expectedUpstreamObj.TokenRequest))
	g.Expect(route.Spec.Upstreams[0].GrantType).To(Equal(expectedUpstreamObj.GrantType))
	g.Expect(route.Spec.Upstreams[0].Scopes).To(Equal(expectedUpstreamObj.Scopes))
}
