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
				Host:     "http://test-redis.de/",
				Port:     123,
				Password: "test-redis-password",
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
			Host:     "http://test-redis.de/",
			Port:     123,
			Password: "test-redis-password",
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
		Url:              "https://test-stargate.de/",
		IssuerUrl:        "https://test-iris.de/auth/realms/test",
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
		Url:              "https://test-stargate.de/",
		IssuerUrl:        "https://test-iris.de/auth/realms/team-test",
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
	g.Expect(zone.Status.Links.GatewayIssuer).To(Equal("https://test-iris.de/auth/realms/test"))
	g.Expect(zone.Status.Links.GatewayUrl).To(Equal("https://test-stargate.de/"))
	g.Expect(zone.Status.Links.StargateLmsIssuer).To(Equal("https://test-stargate.de:443/auth/realms/test"))

	g.Expect(zone.Status.Conditions).To(HaveLen(2))
	g.Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(zone.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
