// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newZone(name string) *adminv1.Zone {
	gatewayAdminSecret := "test-gateway-admin-secret"
	identityAdminUrl := "https://test-iris.de/auth/admin/realms"

	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: adminv1.ZoneSpec{
			IdentityProvider: adminv1.IdentityProviderConfig{
				Admin: adminv1.IdentityProviderAdminConfig{
					Url:      &identityAdminUrl,
					ClientId: "test-idp-admin-id",
					UserName: "test-idp-admin-username",
					Password: "test-idp-admin-password",
				},
				Url: "https://test-iris.de/",
			},
			Gateway: adminv1.GatewayConfig{
				Admin: adminv1.GatewayAdminConfig{
					ClientSecret: &gatewayAdminSecret,
					Url:          "https://test-stargate.de/admin-api",
				},
				Presets: []adminv1.GatewayConfigPreset{
					{
						Name:    "default",
						Default: true,
						Urls: []adminv1.UrlConfig{
							{
								Hostname: "test-stargate.de",
								Scheme:   "https",
								BasePath: "/",
							},
						},
					},
				},
			},
			Redis: &adminv1.RedisConfig{
				Host:      "http://test-redis.de/",
				Port:      123,
				Password:  "test-redis-password",
				EnableTLS: true,
			},
			Visibility: adminv1.ZoneVisibilityWorld,
		},
	}
}

var _ = Describe("Zone Controller", func() {
	Context("When reconciling a zone", func() {
		It("should reach Ready and populate status", func() {
			zone := newZone("smoke-zone")
			zone.Spec.ManagedRoutes = &adminv1.ManagedRoutesConfig{
				Routes: []adminv1.ManagedRouteConfig{{
					Name: "team-api",
					Path: "/team/v1",
					Url:  "https://team.de/v1",
					Type: adminv1.ManagedRouteTypeTeamAPI,
				}},
			}

			By("creating the Environment prerequisite")
			env := &adminv1.Environment{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: testEnvironment, Namespace: testEnvironment}, env)
			if errors.IsNotFound(err) {
				env = &adminv1.Environment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testEnvironment,
						Namespace: testEnvironment,
						Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
					},
					Spec: adminv1.EnvironmentSpec{RealmName: testEnvironment},
				}
				Expect(k8sClient.Create(ctx, env)).To(Succeed())
			}

			By("creating the Zone resource")
			Expect(k8sClient.Create(ctx, zone)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
			})

			By("waiting for the zone to become Ready")
			Eventually(func(g Gomega) {
				got := &adminv1.Zone{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), got)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				By("verifying the namespace was created")
				VerifyNamespace(ctx, g, got.Status.Namespace)

				By("verifying core status references are populated")
				g.Expect(got.Status.Namespace).NotTo(BeEmpty())
				g.Expect(got.Status.IdentityProvider).NotTo(BeNil())
				g.Expect(got.Status.IdentityRealm).NotTo(BeNil())
				g.Expect(got.Status.InternalIdentityRealm).NotTo(BeNil())
				g.Expect(got.Status.Gateway).NotTo(BeNil())
				g.Expect(got.Status.GatewayAdminClient).NotTo(BeNil())
				g.Expect(got.Status.GatewayConsumer).NotTo(BeNil())
				g.Expect(got.Status.TeamApiIdentityRealm).NotTo(BeNil())
				g.Expect(got.Status.ManagedRoutes).NotTo(BeEmpty())

				By("verifying links are populated")
				g.Expect(got.Status.Links.Url).NotTo(BeEmpty())
				g.Expect(got.Status.Links.Issuer).NotTo(BeEmpty())
				g.Expect(got.Status.Links.LmsIssuer).NotTo(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When environment name differs from realmName (decoupling)", func() {
		const (
			decoupledEnvName   = "playground"
			decoupledRealmName = "myrealm"
		)

		It("should create realm CRs with spec.realmName, not metadata.name", func() {
			By("creating the decoupled environment namespace and resource")
			envNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: decoupledEnvName}}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, envNs))).To(Succeed())

			env := &adminv1.Environment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      decoupledEnvName,
					Namespace: decoupledEnvName,
					Labels: map[string]string{
						config.EnvironmentLabelKey: decoupledEnvName,
					},
				},
				Spec: adminv1.EnvironmentSpec{
					RealmName: decoupledRealmName,
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, env))).To(Succeed())

			By("creating a zone in the decoupled environment")
			zone := newZone("zone-decoupled")
			zone.Labels[config.EnvironmentLabelKey] = decoupledEnvName
			zone.Spec.ManagedRoutes = nil // simplify
			Expect(k8sClient.Create(ctx, zone)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, zone))).To(Succeed())
			})

			Eventually(func(g Gomega) {
				got := &adminv1.Zone{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(zone), got)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())

				expectedNs := decoupledEnvName + "--zone-decoupled"

				// Identity Realm named after realmName, not env name
				identityRealm := &identityv1.Realm{}
				err = k8sClient.Get(ctx, client.ObjectKey{Namespace: expectedNs, Name: decoupledRealmName}, identityRealm)
				g.Expect(err).NotTo(HaveOccurred(), "identity realm should be named %q", decoupledRealmName)
				g.Expect(identityRealm.Labels[config.EnvironmentLabelKey]).To(Equal(decoupledEnvName),
					"environment label should be the env name, not the realm name")

				// Issuer URLs contain the realmName, not the environment name
				g.Expect(got.Status.Links.Issuer).To(ContainSubstring("/auth/realms/" + decoupledRealmName))
				g.Expect(got.Status.Links.LmsIssuer).To(ContainSubstring("/auth/realms/" + decoupledRealmName))
			}, timeout, interval).Should(Succeed())
		})
	})
})
