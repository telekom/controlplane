// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"

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
			Redis: adminv1.RedisConfig{
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
					Spec: adminv1.EnvironmentSpec{},
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
				g.Expect(got.Status.GatewayClient).NotTo(BeNil())
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
})
