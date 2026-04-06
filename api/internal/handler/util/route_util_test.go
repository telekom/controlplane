// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Route Util", func() {

	Describe("GatewayConsumerName", func() {
		It("should equal 'gateway'", func() {
			Expect(GatewayConsumerName).To(Equal("gateway"))
		})
	})

	Describe("MakeRouteName", func() {
		It("should return normalized api base path without realm prefix", func() {
			Expect(MakeRouteName("/my/api/v1", "default")).To(Equal("my-api-v1"))
			Expect(MakeRouteName("/my/api/v1", "dtc")).To(Equal("my-api-v1"))
			Expect(MakeRouteName("/my/api/v1", "test")).To(Equal("my-api-v1"))
		})
	})

	Describe("CreateRouteOptions", func() {

		Describe("WithProxyTarget", func() {
			It("should set IsProxyTarget to true", func() {
				opts := &CreateRouteOptions{}
				WithProxyTarget(true)(opts)
				Expect(opts.IsProxyTarget).To(BeTrue())
			})

			It("should set IsProxyTarget to false", func() {
				opts := &CreateRouteOptions{}
				WithProxyTarget(false)(opts)
				Expect(opts.IsProxyTarget).To(BeFalse())
			})
		})

		Describe("WithFailoverUpstreams", func() {
			It("should set failover upstreams on options", func() {
				upstreams := []apiapi.Upstream{
					{Url: "http://upstream1:8080", Weight: 50},
					{Url: "http://upstream2:8080", Weight: 50},
				}

				opts := &CreateRouteOptions{}
				WithFailoverUpstreams(upstreams...)(opts)

				Expect(opts.FailoverUpstreams).To(HaveLen(2))
				Expect(opts.FailoverUpstreams[0].Url).To(Equal("http://upstream1:8080"))
				Expect(opts.FailoverUpstreams[1].Url).To(Equal("http://upstream2:8080"))
			})
		})

		Describe("WithFailoverZone", func() {
			It("should set failover zone on options", func() {
				zone := types.ObjectRef{Name: "my-zone", Namespace: "my-ns"}

				opts := &CreateRouteOptions{}
				WithFailoverZone(zone)(opts)

				Expect(opts.FailoverZone.Name).To(Equal("my-zone"))
				Expect(opts.FailoverZone.Namespace).To(Equal("my-ns"))
			})
		})

		Describe("WithFailoverSecurity", func() {
			It("should set failover security on options", func() {
				security := &apiapi.Security{
					M2M: &apiapi.Machine2MachineAuthentication{
						Scopes: []string{"scope1"},
					},
				}

				opts := &CreateRouteOptions{}
				WithFailoverSecurity(security)(opts)

				Expect(opts.FailoverSecurity).NotTo(BeNil())
				Expect(opts.FailoverSecurity.M2M.Scopes).To(ConsistOf("scope1"))
			})
		})

		Describe("ReturnReferenceOnly", func() {
			It("should set ReturnReferenceOnly to true", func() {
				opts := &CreateRouteOptions{}
				ReturnReferenceOnly()(opts)

				Expect(opts.ReturnReferenceOnly).To(BeTrue())
			})
		})

		Describe("WithServiceRateLimit", func() {
			It("should set service rate limit on options", func() {
				rateLimit := &apiapi.RateLimitConfig{
					Limits: apiapi.Limits{Second: 100, Minute: 1000, Hour: 10000},
				}

				opts := &CreateRouteOptions{}
				WithServiceRateLimit(rateLimit)(opts)

				Expect(opts.ServiceRateLimit).NotTo(BeNil())
				Expect(opts.ServiceRateLimit.Limits.Second).To(Equal(100))
				Expect(opts.ServiceRateLimit.Limits.Minute).To(Equal(1000))
				Expect(opts.ServiceRateLimit.Limits.Hour).To(Equal(10000))
			})
		})

		Describe("WithConsumerRateLimit", func() {
			It("should set consumer rate limit on options", func() {
				limits := &apiapi.Limits{Second: 10, Minute: 100, Hour: 1000}

				opts := &CreateRouteOptions{}
				WithConsumerRateLimit(limits)(opts)

				Expect(opts.ConsumerRateLimit).NotTo(BeNil())
				Expect(opts.ConsumerRateLimit.Second).To(Equal(10))
			})
		})

		Describe("IsFailoverSecondary", func() {
			It("should return true when failover upstreams are set", func() {
				opts := CreateRouteOptions{
					FailoverUpstreams: []apiapi.Upstream{{Url: "http://test:8080"}},
				}
				Expect(opts.IsFailoverSecondary()).To(BeTrue())
			})

			It("should return false when no failover upstreams", func() {
				opts := CreateRouteOptions{}
				Expect(opts.IsFailoverSecondary()).To(BeFalse())
			})
		})

		Describe("HasFailover", func() {
			It("should return true when failover zone is fully set", func() {
				opts := CreateRouteOptions{
					FailoverZone: types.ObjectRef{Name: "zone", Namespace: "ns"},
				}
				Expect(opts.HasFailover()).To(BeTrue())
			})

			It("should return false when failover zone name is empty", func() {
				opts := CreateRouteOptions{
					FailoverZone: types.ObjectRef{Namespace: "ns"},
				}
				Expect(opts.HasFailover()).To(BeFalse())
			})

			It("should return false when failover zone namespace is empty", func() {
				opts := CreateRouteOptions{
					FailoverZone: types.ObjectRef{Name: "zone"},
				}
				Expect(opts.HasFailover()).To(BeFalse())
			})

			It("should return false when failover zone is empty", func() {
				opts := CreateRouteOptions{}
				Expect(opts.HasFailover()).To(BeFalse())
			})
		})

		Describe("HasServiceRateLimit", func() {
			It("should return true when service rate limit is set", func() {
				opts := CreateRouteOptions{ServiceRateLimit: &apiapi.RateLimitConfig{}}
				Expect(opts.HasServiceRateLimit()).To(BeTrue())
			})

			It("should return false when service rate limit is nil", func() {
				opts := CreateRouteOptions{}
				Expect(opts.HasServiceRateLimit()).To(BeFalse())
			})
		})
	})

	Describe("CreateConsumeRouteOptions", func() {

		Describe("WithConsumerRouteRateLimit", func() {
			It("should set consumer rate limit on options", func() {
				limits := apiapi.Limits{Second: 5, Minute: 50, Hour: 500}

				opts := &CreateConsumeRouteOptions{}
				WithConsumerRouteRateLimit(limits)(opts)

				Expect(opts.ConsumerRateLimit).NotTo(BeNil())
				Expect(opts.ConsumerRateLimit.Second).To(Equal(5))
				Expect(opts.ConsumerRateLimit.Minute).To(Equal(50))
				Expect(opts.ConsumerRateLimit.Hour).To(Equal(500))
			})
		})

		Describe("HasConsumerRateLimit", func() {
			It("should return true when consumer rate limit is set", func() {
				opts := CreateConsumeRouteOptions{ConsumerRateLimit: &apiapi.Limits{}}
				Expect(opts.HasConsumerRateLimit()).To(BeTrue())
			})

			It("should return false when consumer rate limit is nil", func() {
				opts := CreateConsumeRouteOptions{}
				Expect(opts.HasConsumerRateLimit()).To(BeFalse())
			})
		})
	})
})
