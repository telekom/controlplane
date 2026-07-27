// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"

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
								Hostname: fmt.Sprintf("test.%s.de", name),
								Scheme:   "http",
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
		Status: adminapi.ZoneStatus{},
	}

	err := k8sClient.Create(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	zone.SetCondition(condition.NewReadyCondition("Ready", "testing"))
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
	err = k8sClient.Status().Update(ctx, zone)
	Expect(err).ToNot(HaveOccurred())

	CreateNamespace(zone.Status.Namespace)
	return zone
}

var _ = Describe("Util Tests", func() {
	Context("Creation of Proxy-Routes", Ordered, func() {
		ctx = context.Background()
		var consumerZone *adminapi.Zone
		var providerZone *adminapi.Zone

		BeforeAll(func() {
			By("Injecting the context-info")
			ctx = client.WithClient(ctx, client.NewJanitorClient(client.NewScopedClient(k8sClient, testEnvironment)))
			ctx = contextutil.WithEnv(ctx, testEnvironment)

			By("Creating the consumer Zone")
			consumerZone = CreateZone("consumer")

			By("Creating the provider Zone")
			providerZone = CreateZone("provider")
		})

		It("should create a normal Proxy-Route", func() {
			By("Creating the Proxy-Route")
			route, err := util.CreateProxyRoute(ctx, *types.ObjectRefFromObject(consumerZone), *types.ObjectRefFromObject(providerZone), "/api/test/v1")
			Expect(err).ToNot(HaveOccurred())
			Expect(route).ToNot(BeNil())

			Expect(route.Name).To(Equal("api-test-v1"))
			Expect(route.Namespace).To(Equal(consumerZone.Status.Namespace))

			By("Checking the Route hostnames and paths")
			Expect(route.Spec.Hostnames).To(ContainElement("test.consumer.de"))
			Expect(route.Spec.Paths).To(ContainElement("/api/test/v1"))

			By("Checking the upstream")
			Expect(route.Spec.Backend.Upstreams).ToNot(BeEmpty())
			upstream := route.Spec.Backend.Upstreams[0]
			Expect(upstream.Hostname).To(Equal("test.provider.de"))
			Expect(upstream.Path).To(Equal("/api/test/v1"))
		})
	})
})
