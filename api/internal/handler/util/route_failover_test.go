// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Route Failover", func() {
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

	Describe("Failover scenario: Primary + Secondary + Proxy route topology", func() {
		// This test validates the complete failover topology:
		//
		//  Consumer (Zone A) ---> [Proxy Route in Zone A] ---> [Primary Route in Zone B] ---> Provider API
		//                                                  |
		//                                            if primary down
		//                                                  |
		//                                                  v
		//                                     [Secondary Route in Zone C] ---> Provider API (failover)
		//

		var (
			primaryRoute   *gatewayapi.Route
			secondaryRoute *gatewayapi.Route
			proxyRoute     *gatewayapi.Route
		)

		const (
			apiBasePath        = "/my/api/v1"
			providerZoneName   = "provider-zone"
			failoverZoneName   = "failover-zone"
			consumerZoneName   = "consumer-zone"
			providerGatewayUrl = "http://my-gateway.provider-zone:8080"
			failoverGatewayUrl = "http://my-gateway.failover-zone:8080"
			realBackendUrl     = "http://real-backend:8080/my/api/v1"
			providerIssuer     = "https://idp.provider-zone.example.com"
			consumerIssuer     = "https://idp.consumer-zone.example.com"
			failoverIssuer     = "https://idp.failover-zone.example.com"
			consumerLmsIssuer  = "https://lms.consumer-zone.example.com"
			failoverLmsIssuer  = "https://lms.failover-zone.example.com"
		)

		BeforeEach(func() {
			// --- Primary Route (Real Route) ---
			// Lives in provider-zone, points directly to the real API backend.
			// This is what CreateRealRoute produces.
			primaryRoute = &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      labelutil.NormalizeNameValue(MakeRouteName(apiBasePath)),
					Namespace: providerZoneName + "-ns",
					Labels: map[string]string{
						apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(apiBasePath),
						config.BuildLabelKey("zone"): labelutil.NormalizeValue(providerZoneName),
						config.BuildLabelKey("type"): "real",
					},
				},
				Spec: gatewayapi.RouteSpec{
					Type: gatewayapi.RouteTypePrimary,
					Backend: gatewayapi.Backend{
						Upstreams: []gatewayapi.Upstream{
							{Scheme: "http", Hostname: "real-backend", Port: 8080, Path: "/my/api/v1"},
						},
					},
					Hostnames: []string{"my-gateway." + providerZoneName},
					Paths:     []string{apiBasePath},
					Security: gatewayapi.Security{
						DefaultConsumers: []string{GatewayConsumerName},
						TrustedIssuers:   []string{providerIssuer, consumerLmsIssuer, failoverLmsIssuer},
						RealmName:        "test-env",
					},
				},
			}

			// --- Secondary Route ---
			// Lives in failover-zone. Its backend points to the provider-zone gateway (meshing),
			// and its failover config points to the real API backend (direct access when primary is down).
			secondaryRoute = &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      labelutil.NormalizeNameValue(MakeRouteName(apiBasePath)),
					Namespace: failoverZoneName + "-ns",
					Labels: map[string]string{
						apiapi.BasePathLabelKey:      labelutil.NormalizeLabelValue(apiBasePath),
						config.BuildLabelKey("zone"): labelutil.NormalizeValue(failoverZoneName),
						config.BuildLabelKey("type"): "proxy",
						LabelFailoverSecondary:       labelTrue,
					},
				},
				Spec: gatewayapi.RouteSpec{
					Type: gatewayapi.RouteTypeSecondary,
					Backend: gatewayapi.Backend{
						Upstreams: []gatewayapi.Upstream{
							{Scheme: "http", Hostname: "my-gateway.provider-zone", Port: 8080, Path: "/my/api/v1"},
						},
					},
					Hostnames: []string{"my-gateway." + failoverZoneName},
					Paths:     []string{apiBasePath},
					Security: gatewayapi.Security{
						DefaultConsumers: []string{GatewayConsumerName},
						TrustedIssuers:   []string{failoverIssuer, consumerLmsIssuer},
						RealmName:        "test-env",
					},
					Traffic: gatewayapi.Traffic{
						Failover: &gatewayapi.Failover{
							TargetZoneName: providerZoneName,
							Upstreams: []gatewayapi.Upstream{
								{Scheme: "http", Hostname: "real-backend", Port: 8080, Path: "/my/api/v1"},
							},
						},
					},
				},
			}

			// --- Proxy Route ---
			// Lives in consumer-zone. Its backend points to the provider-zone gateway.
			// Its failover config points to the failover-zone gateway (secondary route).
			proxyRoute = &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      labelutil.NormalizeNameValue(MakeRouteName(apiBasePath)),
					Namespace: consumerZoneName + "-ns",
					Labels: map[string]string{
						apiapi.BasePathLabelKey:               labelutil.NormalizeLabelValue(apiBasePath),
						config.BuildLabelKey("zone"):          labelutil.NormalizeValue(consumerZoneName),
						config.BuildLabelKey("type"):          "proxy",
						config.BuildLabelKey("failover.zone"): labelutil.NormalizeValue(failoverZoneName),
					},
				},
				Spec: gatewayapi.RouteSpec{
					Type: gatewayapi.RouteTypeProxy,
					Backend: gatewayapi.Backend{
						Upstreams: []gatewayapi.Upstream{
							{Scheme: "http", Hostname: "my-gateway.provider-zone", Port: 8080, Path: "/my/api/v1"},
						},
					},
					Hostnames: []string{"my-gateway." + consumerZoneName},
					Paths:     []string{apiBasePath},
					Security: gatewayapi.Security{
						TrustedIssuers: []string{consumerIssuer},
						RealmName:      "test-env",
					},
					Traffic: gatewayapi.Traffic{
						Failover: &gatewayapi.Failover{
							TargetZoneName: providerZoneName,
							Upstreams: []gatewayapi.Upstream{
								{Scheme: "http", Hostname: "my-gateway.failover-zone", Port: 8080, Path: "/my/api/v1"},
							},
						},
					},
				},
			}
		})

		Context("Primary route validation", func() {
			It("should be of type primary", func() {
				Expect(primaryRoute.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
			})

			It("should be labeled as a 'real' route", func() {
				Expect(primaryRoute.Labels[config.BuildLabelKey("type")]).To(Equal("real"))
			})

			It("should point directly to the real backend", func() {
				Expect(primaryRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(primaryRoute.Spec.Backend.Upstreams[0].Url()).To(Equal(realBackendUrl))
			})

			It("should include 'gateway' in DefaultConsumers for cross-zone proxy access", func() {
				Expect(primaryRoute.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))
			})

			It("should trust issuers from all proxy zones (LMS issuers)", func() {
				Expect(primaryRoute.Spec.Security.TrustedIssuers).To(ContainElements(
					consumerLmsIssuer,
					failoverLmsIssuer,
				))
			})

			It("should live in the provider zone namespace", func() {
				Expect(primaryRoute.Namespace).To(Equal(providerZoneName + "-ns"))
			})
		})

		Context("Secondary route validation", func() {
			It("should be of type secondary", func() {
				Expect(secondaryRoute.Spec.Type).To(Equal(gatewayapi.RouteTypeSecondary))
			})

			It("should be labeled as failover secondary", func() {
				Expect(secondaryRoute.Labels[LabelFailoverSecondary]).To(Equal(labelTrue))
			})

			It("should have its backend pointing to the provider zone gateway", func() {
				Expect(secondaryRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(secondaryRoute.Spec.Backend.Upstreams[0].Url()).To(Equal(providerGatewayUrl + apiBasePath))
			})

			It("should have failover upstreams pointing to the real backend", func() {
				Expect(secondaryRoute.Spec.Traffic.Failover).ToNot(BeNil())
				Expect(secondaryRoute.Spec.Traffic.Failover.Upstreams).To(HaveLen(1))
				Expect(secondaryRoute.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal(realBackendUrl))
			})

			It("should have TargetZoneName set to the provider zone", func() {
				Expect(secondaryRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZoneName))
			})

			It("should include 'gateway' in DefaultConsumers for proxy route access", func() {
				Expect(secondaryRoute.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))
			})

			It("should live in the failover zone namespace", func() {
				Expect(secondaryRoute.Namespace).To(Equal(failoverZoneName + "-ns"))
			})
		})

		Context("Proxy route validation", func() {
			It("should be of type proxy", func() {
				Expect(proxyRoute.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
			})

			It("should not be labeled as failover secondary", func() {
				Expect(proxyRoute.Labels[LabelFailoverSecondary]).To(BeEmpty())
			})

			It("should be labeled with the failover zone", func() {
				Expect(proxyRoute.Labels[config.BuildLabelKey("failover.zone")]).To(Equal(labelutil.NormalizeValue(failoverZoneName)))
			})

			It("should have its backend pointing to the provider zone gateway", func() {
				Expect(proxyRoute.Spec.Backend.Upstreams).To(HaveLen(1))
				Expect(proxyRoute.Spec.Backend.Upstreams[0].Url()).To(Equal(providerGatewayUrl + apiBasePath))
			})

			It("should have failover upstreams pointing to the failover zone gateway", func() {
				Expect(proxyRoute.Spec.Traffic.Failover).ToNot(BeNil())
				Expect(proxyRoute.Spec.Traffic.Failover.Upstreams).To(HaveLen(1))
				Expect(proxyRoute.Spec.Traffic.Failover.Upstreams[0].Url()).To(Equal(failoverGatewayUrl + apiBasePath))
			})

			It("should have TargetZoneName set to the provider zone for health checking", func() {
				Expect(proxyRoute.Spec.Traffic.Failover.TargetZoneName).To(Equal(providerZoneName))
			})

			It("should trust only the consumer zone issuer", func() {
				Expect(proxyRoute.Spec.Security.TrustedIssuers).To(ConsistOf(consumerIssuer))
			})

			It("should live in the consumer zone namespace", func() {
				Expect(proxyRoute.Namespace).To(Equal(consumerZoneName + "-ns"))
			})
		})

		Context("Topology relationships", func() {
			It("should have the proxy route backend pointing to the same zone as the primary route", func() {
				// The proxy route forwards traffic to the provider zone where the primary route lives
				primaryZone := primaryRoute.Labels[config.BuildLabelKey("zone")]
				proxyBackendHostname := proxyRoute.Spec.Backend.Upstreams[0].Hostname
				Expect(proxyBackendHostname).To(ContainSubstring(primaryZone))
			})

			It("should have the proxy route failover pointing to the same zone as the secondary route", func() {
				// When primary is down, proxy route redirects to the failover zone where secondary route lives
				secondaryZone := secondaryRoute.Labels[config.BuildLabelKey("zone")]
				proxyFailoverHostname := proxyRoute.Spec.Traffic.Failover.Upstreams[0].Hostname
				Expect(proxyFailoverHostname).To(ContainSubstring(secondaryZone))
			})

			It("should have the secondary route failover pointing to the real backend (same as primary route backend)", func() {
				// The secondary route's failover upstreams go directly to the real API
				Expect(secondaryRoute.Spec.Traffic.Failover.Upstreams[0].Url()).To(
					Equal(primaryRoute.Spec.Backend.Upstreams[0].Url()),
				)
			})

			It("should have all three routes serving the same base path", func() {
				Expect(primaryRoute.Spec.Paths).To(ContainElement(apiBasePath))
				Expect(secondaryRoute.Spec.Paths).To(ContainElement(apiBasePath))
				Expect(proxyRoute.Spec.Paths).To(ContainElement(apiBasePath))
			})

			It("should have primary and secondary routes accept gateway consumer traffic", func() {
				// Both primary and secondary are targets of proxy routes,
				// so they must allow the gateway mesh-client
				Expect(primaryRoute.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))
				Expect(secondaryRoute.Spec.Security.DefaultConsumers).To(ContainElement(GatewayConsumerName))
			})

			It("should have the proxy route NOT include gateway in default consumers", func() {
				// The proxy route receives end-user traffic, not mesh traffic
				Expect(proxyRoute.Spec.Security.DefaultConsumers).ToNot(ContainElement(GatewayConsumerName))
			})
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
