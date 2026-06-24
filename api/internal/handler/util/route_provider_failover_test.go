// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Route Provider Failover", func() {
	Describe("configureAsFailoverTarget", func() {
		var (
			ctx        context.Context
			proxyRoute *gatewayapi.Route
		)

		BeforeEach(func() {
			ctx = context.Background()
			proxyRoute = &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-v1",
					Namespace: "failover-zone-ns",
					Labels: map[string]string{
						config.BuildLabelKey("type"): "proxy",
					},
				},
				Spec: gatewayapi.RouteSpec{
					Type: gatewayapi.RouteTypeProxy,
					Backend: gatewayapi.Backend{
						Upstreams: []gatewayapi.Upstream{
							{Scheme: "http", Hostname: "my-gateway.provider-zone", Port: 8080, Path: "/my/api/v1"},
						},
					},
					Hostnames: []string{"my-gateway.failover-zone"},
					Paths:     []string{"/my/api/v1"},
				},
			}
		})

		It("should set the failover.secondary label to true and type to secondary", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Labels[LabelFailoverSecondary]).To(Equal("true"))
			Expect(proxyRoute.Spec.Type).To(Equal(gatewayapi.RouteTypeSecondary))
		})

		It("should add 'gateway' to DefaultConsumers", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))
		})

		It("should set the failover upstreams correctly", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Traffic.Failover).ToNot(BeNil())
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams).To(HaveLen(1))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Hostname).To(Equal("real-backend"))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Port).To(Equal(int32(8080)))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Path).To(Equal("/my/api/v1"))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Scheme).To(Equal("http"))
		})

		It("should set the TargetZoneName to the upstream zone name", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal("provider-zone"))
		})

		It("should handle multiple failover upstreams for load balancing", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://backend-a:8080/my/api/v1", Weight: 60},
					{Url: "http://backend-b:9090/my/api/v1", Weight: 40},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams).To(HaveLen(2))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Hostname).To(Equal("backend-a"))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Weight).To(Equal(int32(60)))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[1].Hostname).To(Equal("backend-b"))
			Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[1].Weight).To(Equal(int32(40)))
		})

		It("should append trusted issuers when provided", func() {
			proxyRoute.Spec.Security.TrustedIssuers = []string{"https://zone-issuer.example.com"}
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
				TrustedIssuers: []string{"https://lms-issuer-a.example.com", "https://lms-issuer-b.example.com"},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Security.TrustedIssuers).To(HaveLen(3))
			Expect(proxyRoute.Spec.Security.TrustedIssuers).To(ContainElements(
				"https://zone-issuer.example.com",
				"https://lms-issuer-a.example.com",
				"https://lms-issuer-b.example.com",
			))
		})

		It("should not modify trusted issuers when none are provided", func() {
			proxyRoute.Spec.Security.TrustedIssuers = []string{"https://zone-issuer.example.com"}
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Security.TrustedIssuers).To(HaveLen(1))
			Expect(proxyRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://zone-issuer.example.com"))
		})

		It("should copy failover security with ExternalIDP when provided", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
				FailoverSecurity: &apiapi.Security{
					M2M: &apiapi.Machine2MachineAuthentication{
						ExternalIDP: &apiapi.ExternalIdentityProvider{
							TokenEndpoint: "https://idp.example.com/token",
							GrantType:     "client_credentials",
							Client: &apiapi.OAuth2ClientCredentials{
								ClientId:     "my-client-id",
								ClientSecret: "my-client-secret",
							},
						},
						Scopes: []string{"read", "write"},
					},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M).ToNot(BeNil())
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP).ToNot(BeNil())
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("my-client-id"))
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.Scopes).To(ConsistOf("read", "write"))
		})

		It("should copy failover security with BasicAuth when provided", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
				FailoverSecurity: &apiapi.Security{
					M2M: &apiapi.Machine2MachineAuthentication{
						Basic: &apiapi.BasicAuthCredentials{
							Username: "admin",
							Password: "secret",
						},
					},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M).ToNot(BeNil())
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.Basic).ToNot(BeNil())
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.Basic.Username).To(Equal("admin"))
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M.Basic.Password).To(Equal("secret"))
		})

		It("should not set failover security when nil", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
				FailoverSecurity: nil,
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			// Security should be zero-value (no M2M set)
			Expect(proxyRoute.Spec.Traffic.Failover.Security.M2M).To(BeNil())
		})

		It("should return an error for an invalid upstream URL", func() {
			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "://invalid-url", Weight: 100},
				},
			}

			err := configureAsFailoverTarget(ctx, proxyRoute, options, "provider-zone")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create failover upstream"))
		})
	})

	Describe("configureAsFailoverTarget produces a valid secondary route from a proxy route", func() {
		It("should transform a basic proxy route into a secondary route with full failover config", func() {
			ctx := context.Background()

			// Start with a basic proxy route (as CreateProxyRoute would initialize it)
			route := &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-api-v1",
					Namespace: "failover-zone-ns",
					Labels: map[string]string{
						config.BuildLabelKey("type"): "proxy",
						config.BuildLabelKey("zone"): "failover-zone",
					},
				},
				Spec: gatewayapi.RouteSpec{
					Type: gatewayapi.RouteTypeProxy,
					Backend: gatewayapi.Backend{
						Upstreams: []gatewayapi.Upstream{
							{Scheme: "http", Hostname: "my-gateway.provider-zone", Port: 8080, Path: "/my/api/v1"},
						},
					},
					Hostnames: []string{"my-gateway.failover-zone"},
					Paths:     []string{"/my/api/v1"},
					Security: gatewayapi.Security{
						TrustedIssuers: []string{"https://idp.failover-zone.example.com"},
					},
				},
			}

			options := &CreateRouteOptions{
				FailoverUpstreams: []apiapi.Upstream{
					{Url: "http://real-backend:8080/my/api/v1", Weight: 100},
				},
				TrustedIssuers: []string{"https://lms.consumer-zone.example.com"},
				FailoverSecurity: &apiapi.Security{
					M2M: &apiapi.Machine2MachineAuthentication{
						ExternalIDP: &apiapi.ExternalIdentityProvider{
							TokenEndpoint: "https://idp.example.com/token",
							GrantType:     "client_credentials",
							Client: &apiapi.OAuth2ClientCredentials{
								ClientId:     "failover-client",
								ClientSecret: "failover-secret",
							},
						},
						Scopes: []string{"api.read"},
					},
				},
			}

			err := configureAsFailoverTarget(ctx, route, options, "provider-zone")
			Expect(err).ToNot(HaveOccurred())

			// Verify it is now a secondary route
			Expect(route.Labels[LabelFailoverSecondary]).To(Equal("true"))
			Expect(route.Spec.Type).To(Equal(gatewayapi.RouteTypeSecondary))

			// Verify gateway consumer is added
			Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))

			// Verify trusted issuers are merged
			Expect(route.Spec.Security.TrustedIssuers).To(ContainElements(
				"https://idp.failover-zone.example.com",
				"https://lms.consumer-zone.example.com",
			))

			// Verify failover traffic config
			Expect(route.Spec.Traffic.Failover).ToNot(BeNil())
			Expect(route.Spec.Traffic.Failover.TargetZoneName).To(Equal("provider-zone"))
			Expect(route.Spec.Traffic.Failover.Upstreams).To(HaveLen(1))
			Expect(route.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal("http://real-backend:8080/my/api/v1"))

			// Verify failover security
			Expect(route.Spec.Traffic.Failover.Security.M2M).ToNot(BeNil())
			Expect(route.Spec.Traffic.Failover.Security.M2M.ExternalIDP).ToNot(BeNil())
			Expect(route.Spec.Traffic.Failover.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
			Expect(route.Spec.Traffic.Failover.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("failover-client"))
			Expect(route.Spec.Traffic.Failover.Security.M2M.Scopes).To(ConsistOf("api.read"))
		})
	})
})
