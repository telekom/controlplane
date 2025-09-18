// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityapi "github.com/telekom/controlplane/identity/api/v1"
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

func CreateRealm(name, zoneName string) *gatewayapi.Realm {
	gw := &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testEnvironment + "--" + zoneName,
			Labels: map[string]string{
				config.EnvironmentLabelKey:   testEnvironment,
				config.BuildLabelKey("zone"): zoneName,
			},
		},
		Spec: gatewayapi.RealmSpec{
			Url:       fmt.Sprintf("http://%s.%s.de:8080", name, zoneName),
			IssuerUrl: fmt.Sprintf("http://issuer.%s.de:8080/auth/realms/test", zoneName),
		},
	}

	err := k8sClient.Create(ctx, gw)
	Expect(err).ToNot(HaveOccurred())
	return gw
}

func CreateGatewayClient(zone *adminapi.Zone) *identityapi.Client {
	gwClient := &identityapi.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: zone.Status.Namespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: identityapi.ClientSpec{
			Realm: &types.ObjectRef{
				Name:      "test",
				Namespace: zone.Status.Namespace,
			},
			ClientId:     "gateway",
			ClientSecret: "topsecret",
		},
	}

	err := k8sClient.Create(ctx, gwClient)
	Expect(err).ToNot(HaveOccurred())

	gwClient.Status = identityapi.ClientStatus{
		IssuerUrl: fmt.Sprintf("http://issuer.%s.de:8080/auth/realms/test", zone.Name),
	}
	err = k8sClient.Status().Update(ctx, gwClient)
	Expect(err).ToNot(HaveOccurred())

	return gwClient
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
			CreateRealm(testEnvironment, "consumer")
			CreateGatewayClient(consumerZone)
			By("Creating a second Gateway for the consumer Zone")
			CreateRealm("esp", "consumer")

			By("Creating the provider Zone")
			providerZone = CreateZone("provider")
			CreateRealm(testEnvironment, "provider")
			CreateGatewayClient(providerZone)

		})

		It("should create a normal Proxy-Route", func() {
			By("Creating the Proxy-Route")
			route, err := util.CreateProxyRoute(ctx, *types.ObjectRefFromObject(consumerZone), *types.ObjectRefFromObject(providerZone), "/api/test/v1", testEnvironment)
			Expect(err).ToNot(HaveOccurred())
			Expect(route).ToNot(BeNil())

			Expect(route.Name).To(Equal(testEnvironment + "--api-test-v1"))
			Expect(route.Namespace).To(Equal(consumerZone.Status.Namespace))

			By("Checking the Route")
			downstream := route.Spec.Downstreams[0]
			Expect(downstream).ToNot(BeNil())
			Expect(downstream.Host).To(Equal("test.consumer.de"))
			Expect(downstream.Path).To(Equal("/api/test/v1"))

			upstream := route.Spec.Upstreams[0]
			Expect(upstream).ToNot(BeNil())
			Expect(upstream.Host).To(Equal("test.provider.de"))
			Expect(upstream.Path).To(Equal("/api/test/v1"))

			Expect(upstream.IssuerUrl).To(Equal("http://issuer.provider.de:8080/auth/realms/test"))
			Expect(upstream.ClientId).To(Equal("gateway"))
			Expect(upstream.ClientSecret).To(Equal("topsecret"))
		})

		It("should create a Proxy-Route with the correct virtual-host as downstream", func() {
			By("Creating the Proxy-Route")
			route, err := util.CreateProxyRoute(ctx, *types.ObjectRefFromObject(consumerZone), *types.ObjectRefFromObject(consumerZone), "/api/test/v1", "esp")
			Expect(err).ToNot(HaveOccurred())
			Expect(route).ToNot(BeNil())

			Expect(route.Name).To(Equal("esp--api-test-v1"))
			Expect(route.Namespace).To(Equal(consumerZone.Status.Namespace))

			By("Checking the Route")
			downstream := route.Spec.Downstreams[0]
			Expect(downstream).ToNot(BeNil())
			Expect(downstream.Host).To(Equal("esp.consumer.de"))
			Expect(downstream.Path).To(Equal("/api/test/v1"))

			upstream := route.Spec.Upstreams[0]
			Expect(upstream).ToNot(BeNil())
			Expect(upstream.Host).To(Equal("esp.consumer.de"))
			Expect(upstream.Path).To(Equal("/api/test/v1"))

			Expect(upstream.IssuerUrl).To(Equal("http://issuer.consumer.de:8080/auth/realms/test"))
			Expect(upstream.ClientId).To(Equal("gateway"))
			Expect(upstream.ClientSecret).To(Equal("topsecret"))
		})

	})
})
