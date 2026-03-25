// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	identityapi "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewZone(name string, namespace string) *adminv1.Zone {
	idpAdminUrl := "https://test-iris.de/auth/admin/realms"
	gatewayAdminUrl := "https://test-stargate.de/admin-api"

	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},

		Spec: adminv1.ZoneSpec{
			IdentityProvider: adminv1.IdentityProviderConfig{
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &idpAdminUrl,
					ClientId: "test-idp-admin-id",
					UserName: "test-idp-admin-username",
					Password: "test-idp-admin-password",
				},
				Url: "https://test-iris.de/",
			},
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					ClientSecret: "test-gateway-admin-secret",
					Url:          &gatewayAdminUrl,
				},
				Url: "https://test-stargate.de/",
			},
			Redis: adminv1.RedisConfig{
				Host:      "http://test-redis.de/",
				Port:      123,
				Password:  "test-redis-password",
				EnableTLS: true,
			},
			TeamApis: &adminv1.TeamApiConfig{
				Apis: []adminv1.ApiConfig{{
					Name: "test-team-api1",
					Path: "/test/team/api/v1",
					Url:  "https://test-team-api-host.de/test-team-api-v1",
				}},
			},
			Visibility: adminv1.ZoneVisibilityWorld,
		},
	}
}

var _ = Describe("Zone Controller", func() {
	Context("When reconciling a resource", func() {
		const zoneName = "test-zone"

		testZoneRef := client.ObjectKey{
			Name:      zoneName,
			Namespace: testNamespace,
		}

		environmentRef := client.ObjectKey{
			Name:      testEnvironment,
			Namespace: testEnvironment,
		}
		environment := &adminv1.Environment{}

		testZone := NewZone(zoneName, testNamespace)

		BeforeEach(func() {
			By("creating the custom resource for the Kind Environment")
			err := k8sClient.Get(ctx, environmentRef, environment)
			if err != nil && errors.IsNotFound(err) {
				resource := &adminv1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testEnvironment,
						Namespace: testEnvironment,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: adminv1.EnvironmentSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating the custom resource for the Kind Zone")
			existingZone := &adminv1.Zone{}
			err = k8sClient.Get(ctx, testZoneRef, existingZone)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testZone)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &adminv1.Zone{}
			err := k8sClient.Get(ctx, testZoneRef, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Zone")
			Expect(k8sClient.Delete(ctx, testZone)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyZone(ctx, g, testZoneRef, testZone)

				expectedNamespaceName := "test--test-zone"
				VerifyNamespace(ctx, g, expectedNamespaceName)

			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyZone(ctx context.Context, g Gomega, namespacedName client.ObjectKey, zoneToVerify *adminv1.Zone) {
	By("Checking if the Zone is created and all conditions are set")
	zone := &adminv1.Zone{}
	err := k8sClient.Get(ctx, namespacedName, zone)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(zone.Spec).To(Equal(zoneToVerify.Spec))

	// Verify all created sub-resources
	// idp
	// idp realm
	// idp client (gateway)
	// gateway
	// gateway realm
	// gateway consumer
	// team api gateway realm
	// team api gateway route

	// Identity provider
	By("Checking if the Identity provider is created and spec is valid")
	identityProvider := &identityapi.IdentityProvider{}
	identityProviderRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "test-zone",
	}
	err = k8sClient.Get(ctx, identityProviderRef, identityProvider)
	g.Expect(err).NotTo(HaveOccurred())

	identityProviderSpec := &identityapi.IdentityProviderSpec{
		AdminUrl:      "https://test-iris.de/auth/admin/realms",
		AdminClientId: "test-idp-admin-id",
		AdminUserName: "test-idp-admin-username",
		AdminPassword: "test-idp-admin-password",
	}
	g.Expect(identityProvider.Spec).To(Equal(*identityProviderSpec))
	g.Expect(zone.Status.IdentityProvider).To(Equal(types.ObjectRefFromObject(identityProvider)))

	// Identity provider realm
	By("Checking if the identity provider realm is created and spec is valid")
	identityProviderRealm := &identityapi.Realm{}
	identityProviderRealmRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "test",
	}
	err = k8sClient.Get(ctx, identityProviderRealmRef, identityProviderRealm)
	g.Expect(err).NotTo(HaveOccurred())

	identityProviderSpecRealm := &identityapi.RealmSpec{
		IdentityProvider: &types.ObjectRef{
			Name:      "test-zone",
			Namespace: "test--test-zone",
		},
	}
	g.Expect(identityProviderRealm.Spec).To(Equal(*identityProviderSpecRealm))
	g.Expect(zone.Status.IdentityRealm).To(Equal(types.ObjectRefFromObject(identityProviderRealm)))

	// Identity provider client (gateway client)
	By("Checking if the identity provider client (gateway) is created and spec is valid")
	identityProviderClient := &identityapi.Client{}
	identityProviderClientRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "gateway",
	}
	err = k8sClient.Get(ctx, identityProviderClientRef, identityProviderClient)
	g.Expect(err).NotTo(HaveOccurred())

	// IMPORTANT !!! - manually set the password of the client from the cluster, to satisfy matching - todo find a better way
	identityProviderClient.Spec.ClientSecret = "randomly-generated-will-be-ignored"

	identityProviderClientSpec := &identityapi.ClientSpec{
		Realm:        types.ObjectRefFromObject(identityProviderRealm),
		ClientId:     "gateway",
		ClientSecret: "randomly-generated-will-be-ignored",
	}
	g.Expect(identityProviderClient.Spec).To(Equal(*identityProviderClientSpec))
	g.Expect(zone.Status.GatewayClient).To(Equal(types.ObjectRefFromObject(identityProviderClient)))

	// verify the created gateway
	By("Checking if the gateway is created and spec is valid")
	gateway := &gatewayapi.Gateway{}
	gatewayRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "gateway",
	}

	err = k8sClient.Get(ctx, gatewayRef, gateway)
	g.Expect(err).NotTo(HaveOccurred())

	gatewaySpec := gatewayapi.GatewaySpec{
		Redis: gatewayapi.RedisConfig{
			Host:      "http://test-redis.de/",
			Port:      123,
			Password:  "test-redis-password",
			EnableTLS: true,
		},
		Admin: gatewayapi.AdminConfig{
			ClientId:     "rover",
			ClientSecret: "test-gateway-admin-secret",
			IssuerUrl:    "https://test-iris.de/auth/realms/rover",
			Url:          "https://test-stargate.de/admin-api",
		},
		Features: nil,
	}
	g.Expect(gateway.Spec).To(Equal(gatewaySpec))
	g.Expect(zone.Status.Gateway).To(Equal(types.ObjectRefFromObject(gateway)))

	// verify the created gateway realm
	By("Checking if the gateway realm is created and spec is valid")
	gatewayRealm := &gatewayapi.Realm{}
	gatewayRealmRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "test",
	}
	err = k8sClient.Get(ctx, gatewayRealmRef, gatewayRealm)
	g.Expect(err).NotTo(HaveOccurred())

	gatewayRealmSpec := gatewayapi.RealmSpec{
		Gateway:          types.ObjectRefFromObject(gateway),
		Urls:             []string{"https://test-stargate.de/"},
		IssuerUrls:       []string{"https://test-iris.de/auth/realms/test"},
		DefaultConsumers: []string{"gateway"},
	}
	g.Expect(gatewayRealm.Spec).To(Equal(gatewayRealmSpec))
	g.Expect(zone.Status.GatewayRealm).To(Equal(types.ObjectRefFromObject(gatewayRealm)))

	// verify the created gateway consumer
	By("Checking if the gateway consumer is created and spec is valid")
	gatewayConsumer := &gatewayapi.Consumer{}
	gatewayConsumerRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "gateway",
	}
	err = k8sClient.Get(ctx, gatewayConsumerRef, gatewayConsumer)
	g.Expect(err).NotTo(HaveOccurred())

	gatewayConsumerSpec := gatewayapi.ConsumerSpec{
		Realm: *types.ObjectRefFromObject(gatewayRealm),
		Name:  "gateway",
	}
	g.Expect(gatewayConsumer.Spec).To(Equal(gatewayConsumerSpec))
	g.Expect(zone.Status.GatewayConsumer).To(Equal(types.ObjectRefFromObject(gatewayConsumer)))

	// verify the team api gateway realm
	By("Checking if the team api gateway realm is created and spec is valid")
	teamApiGatewayRealm := &gatewayapi.Realm{}
	teamApiGatewayRealmRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "team-test",
	}
	err = k8sClient.Get(ctx, teamApiGatewayRealmRef, teamApiGatewayRealm)
	g.Expect(err).NotTo(HaveOccurred())

	teamApiGatewayRealmSpec := gatewayapi.RealmSpec{
		Gateway:          types.ObjectRefFromObject(gateway),
		Urls:             []string{"https://test-stargate.de/"},
		IssuerUrls:       []string{"https://test-iris.de/auth/realms/team-test"},
		DefaultConsumers: []string{"gateway"},
	}
	g.Expect(teamApiGatewayRealm.Spec).To(Equal(teamApiGatewayRealmSpec))
	g.Expect(zone.Status.TeamApiGatewayRealm).To(Equal(types.ObjectRefFromObject(teamApiGatewayRealm)))

	// verify the team api gateway route
	By("Checking if the team api route is created and spec is valid")
	teamApiRoute := &gatewayapi.Route{}
	teamApiRouteRef := client.ObjectKey{
		Namespace: "test--test-zone",
		Name:      "team-test--test-team-api1",
	}
	err = k8sClient.Get(ctx, teamApiRouteRef, teamApiRoute)
	g.Expect(err).NotTo(HaveOccurred())
	teamApiRouteSpec := gatewayapi.RouteSpec{
		Realm:       *types.ObjectRefFromObject(teamApiGatewayRealm),
		PassThrough: false,
		Upstreams: []gatewayapi.Upstream{
			{
				Scheme: "https",
				Host:   "test-team-api-host.de",
				Port:   443,
				Path:   "/test-team-api-v1",
			},
		},
		Downstreams: []gatewayapi.Downstream{
			{
				Host:      "test-stargate.de",
				Port:      0,
				Path:      "/test/team/api/v1",
				IssuerUrl: "https://test-iris.de/auth/realms/team-test",
			},
		},
		Security: &gatewayapi.Security{
			DisableAccessControl: true,
		},
	}
	g.Expect(teamApiRoute.Spec).To(Equal(teamApiRouteSpec))
	g.Expect(zone.Status.Gateway).To(Equal(types.ObjectRefFromObject(gateway)))

	// verify the links
	By("Checking if the links in the status are created and valid")
	g.Expect(zone.Status.Links.Issuer).To(Equal("https://test-iris.de/auth/realms/test"))
	g.Expect(zone.Status.Links.Url).To(Equal("https://test-stargate.de/"))
	g.Expect(zone.Status.Links.LmsIssuer).To(Equal("https://test-stargate.de/auth/realms/test"))
	g.Expect(zone.Status.Links.TeamIssuer).To(Equal("https://test-iris.de/auth/realms/team-test"))

	g.Expect(zone.Status.Conditions).To(HaveLen(2))
	g.Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}

var _ = Describe("Zone Controller - DTC", func() {
	Context("When reconciling zones with DTC URLs", func() {
		const zone1Name = "zone1"
		const zone2Name = "zone2"
		const zone3Name = "zone3-no-dtc"

		zone1Ref := client.ObjectKey{
			Name:      zone1Name,
			Namespace: testNamespace,
		}
		zone2Ref := client.ObjectKey{
			Name:      zone2Name,
			Namespace: testNamespace,
		}
		zone3Ref := client.ObjectKey{
			Name:      zone3Name,
			Namespace: testNamespace,
		}

		environmentRef := client.ObjectKey{
			Name:      testEnvironment,
			Namespace: testEnvironment,
		}

		BeforeEach(func() {
			By("creating the environment")
			environment := &adminv1.Environment{}
			err := k8sClient.Get(ctx, environmentRef, environment)
			if err != nil && errors.IsNotFound(err) {
				resource := &adminv1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testEnvironment,
						Namespace: testEnvironment,
						Labels: map[string]string{
							config.EnvironmentLabelKey: testEnvironment,
						},
					},
					Spec: adminv1.EnvironmentSpec{},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			By("creating zone1 with DTC URL and unique IDP")
			zone1 := NewZone(zone1Name, testNamespace)
			zone1.Spec.Gateway.DtcUrl = "https://dtc1.example.com/"
			zone1.Spec.IdentityProvider.Url = "https://idp-zone1.example.com/"
			err = k8sClient.Create(ctx, zone1)
			Expect(err).NotTo(HaveOccurred())

			By("creating zone2 with DTC URL and unique IDP")
			zone2 := NewZone(zone2Name, testNamespace)
			zone2.Spec.Gateway.DtcUrl = "https://dtc2.example.com/"
			zone2.Spec.IdentityProvider.Url = "https://idp-zone2.example.com/"
			err = k8sClient.Create(ctx, zone2)
			Expect(err).NotTo(HaveOccurred())

			By("creating zone3 without DTC URL")
			zone3 := NewZone(zone3Name, testNamespace)
			zone3.Spec.IdentityProvider.Url = "https://idp-zone3.example.com/"
			err = k8sClient.Create(ctx, zone3)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("cleanup zones")
			zone1 := &adminv1.Zone{}
			err := k8sClient.Get(ctx, zone1Ref, zone1)
			if err == nil {
				Expect(k8sClient.Delete(ctx, zone1)).To(Succeed())
			}
			zone2 := &adminv1.Zone{}
			err = k8sClient.Get(ctx, zone2Ref, zone2)
			if err == nil {
				Expect(k8sClient.Delete(ctx, zone2)).To(Succeed())
			}
			zone3 := &adminv1.Zone{}
			err = k8sClient.Get(ctx, zone3Ref, zone3)
			if err == nil {
				Expect(k8sClient.Delete(ctx, zone3)).To(Succeed())
			}
		})

		It("should create DTC gateway realms with correct URLs and issuers", func() {
			By("waiting for zone1 to be ready")
			zone1 := &adminv1.Zone{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, zone1Ref, zone1)
				return err == nil && meta.IsStatusConditionTrue(zone1.Status.Conditions, condition.ConditionTypeReady)
			}, timeout, interval).Should(BeTrue())

			By("waiting for zone2 to be ready")
			zone2 := &adminv1.Zone{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, zone2Ref, zone2)
				return err == nil && meta.IsStatusConditionTrue(zone2.Status.Conditions, condition.ConditionTypeReady)
			}, timeout, interval).Should(BeTrue())

			By("waiting for zone3 to be ready")
			zone3 := &adminv1.Zone{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, zone3Ref, zone3)
				return err == nil && meta.IsStatusConditionTrue(zone3.Status.Conditions, condition.ConditionTypeReady)
			}, timeout, interval).Should(BeTrue())

			By("verifying zone1 has DTC gateway realm reference in status")
			Expect(zone1.Status.DtcGatewayRealm).NotTo(BeNil())
			Expect(zone1.Status.DtcGatewayRealm.Name).To(Equal("dtc"))

			By("verifying zone2 has DTC gateway realm reference in status")
			Expect(zone2.Status.DtcGatewayRealm).NotTo(BeNil())
			Expect(zone2.Status.DtcGatewayRealm.Name).To(Equal("dtc"))

			By("verifying zone3 does NOT have DTC gateway realm reference")
			Expect(zone3.Status.DtcGatewayRealm).To(BeNil())

			By("verifying zone1 DTC realm contains all DTC URLs")
			dtcRealm1 := &gatewayapi.Realm{}
			dtcRealm1Ref := client.ObjectKey{
				Namespace: zone1.Status.Namespace,
				Name:      "dtc",
			}
			err := k8sClient.Get(ctx, dtcRealm1Ref, dtcRealm1)
			Expect(err).NotTo(HaveOccurred())

			// Should contain both DTC URLs
			Expect(dtcRealm1.Spec.Urls).To(ContainElements(
				"https://dtc1.example.com/",
				"https://dtc2.example.com/",
			))

			// Should contain issuer from zone2 (OTHER zone with DTC), but not from zone1 (self)
			Expect(dtcRealm1.Spec.IssuerUrls).To(ContainElement("https://idp-zone2.example.com/auth/realms/dtc"))
			// Should have exactly 1 issuer (from zone2 only)
			Expect(dtcRealm1.Spec.IssuerUrls).To(HaveLen(1))

			By("verifying zone2 DTC realm contains all DTC URLs")
			dtcRealm2 := &gatewayapi.Realm{}
			dtcRealm2Ref := client.ObjectKey{
				Namespace: zone2.Status.Namespace,
				Name:      "dtc",
			}
			err = k8sClient.Get(ctx, dtcRealm2Ref, dtcRealm2)
			Expect(err).NotTo(HaveOccurred())

			// Should contain both DTC URLs
			Expect(dtcRealm2.Spec.Urls).To(ContainElements(
				"https://dtc1.example.com/",
				"https://dtc2.example.com/",
			))

			// Should contain issuer from zone1 (OTHER zone with DTC), but not from zone2 (self)
			Expect(dtcRealm2.Spec.IssuerUrls).To(ContainElement("https://idp-zone1.example.com/auth/realms/dtc"))
			// Should have exactly 1 issuer (from zone1 only)
			Expect(dtcRealm2.Spec.IssuerUrls).To(HaveLen(1))

			By("verifying zone3 has no DTC realm created")
			dtcRealm3 := &gatewayapi.Realm{}
			dtcRealm3Ref := client.ObjectKey{
				Namespace: zone3.Status.Namespace,
				Name:      "dtc",
			}
			err = k8sClient.Get(ctx, dtcRealm3Ref, dtcRealm3)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("verifying default gateway realms are NOT affected by DTC")
			defaultRealm1 := &gatewayapi.Realm{}
			defaultRealm1Ref := client.ObjectKey{
				Namespace: zone1.Status.Namespace,
				Name:      testEnvironment,
			}
			err = k8sClient.Get(ctx, defaultRealm1Ref, defaultRealm1)
			Expect(err).NotTo(HaveOccurred())
			// Default realm should only have the regular URL, NOT DTC URLs
			Expect(defaultRealm1.Spec.Urls).To(Equal([]string{"https://test-stargate.de/"}))
		})
	})
})
