// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Exposure Traffic Mapper", func() {
	Context("mapExposureTraffic", func() {
		It("must map failover zones correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				Failover: api.Failover{
					Zones: []string{"zone1", "zone2"},
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.Failover).ToNot(BeNil())
			Expect(output.Traffic.Failover.Zones).To(HaveLen(2))
			Expect(output.Traffic.Failover.Zones).To(ContainElements("zone1", "zone2"))
			snaps.MatchSnapshot(GinkgoT(), output.Traffic)
		})

		It("must handle empty failover zones", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				Failover: api.Failover{
					Zones: []string{},
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.Failover).To(BeNil())
		})

		It("must handle nil failover zones", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				// Failover is not initialized
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.Failover).To(BeNil())
		})

		It("must map provider rate limits correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				RateLimit: api.RateLimitContainer{
					Provider: api.RateLimit{
						Second:            100,
						Minute:            1000,
						Hour:              10000,
						FaultTolerant:     true,
						HideClientHeaders: true,
					},
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.RateLimit).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Provider).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Provider.Limits).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Provider.Limits.Second).To(Equal(100))
			Expect(output.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(1000))
			Expect(output.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(10000))
			Expect(output.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
			Expect(output.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
			snaps.MatchSnapshot(GinkgoT(), output.Traffic.RateLimit)
		})

		It("must map consumer default rate limits correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				RateLimit: api.RateLimitContainer{
					ConsumerDefault: api.RateLimit{
						Second:            50,
						Minute:            500,
						Hour:              5000,
						FaultTolerant:     true,
						HideClientHeaders: true,
					},
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.RateLimit).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers.Default).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers.Default.Limits).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers.Default.Limits.Second).To(Equal(50))
			Expect(output.Traffic.RateLimit.Consumers.Default.Limits.Minute).To(Equal(500))
			Expect(output.Traffic.RateLimit.Consumers.Default.Limits.Hour).To(Equal(5000))
			snaps.MatchSnapshot(GinkgoT(), output.Traffic.RateLimit)
		})

		It("must map consumer overrides correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				RateLimit: api.RateLimitContainer{
					Consumers: []api.ConsumerRateLimit{
						{
							Id:                "consumer1",
							Second:            25,
							Minute:            250,
							Hour:              2500,
							FaultTolerant:     true,
							HideClientHeaders: false,
						},
						{
							Id:                "consumer2",
							Second:            75,
							Minute:            750,
							Hour:              7500,
							FaultTolerant:     false,
							HideClientHeaders: true,
						},
					},
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Traffic.RateLimit).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers).ToNot(BeNil())
			Expect(output.Traffic.RateLimit.Consumers.Overrides).To(HaveLen(2))

			// First consumer
			Expect(output.Traffic.RateLimit.Consumers.Overrides[0].Consumer).To(Equal("consumer1"))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[0].Limits.Second).To(Equal(25))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[0].Limits.Minute).To(Equal(250))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[0].Limits.Hour).To(Equal(2500))

			// Second consumer
			Expect(output.Traffic.RateLimit.Consumers.Overrides[1].Consumer).To(Equal("consumer2"))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[1].Limits.Second).To(Equal(75))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[1].Limits.Minute).To(Equal(750))
			Expect(output.Traffic.RateLimit.Consumers.Overrides[1].Limits.Hour).To(Equal(7500))

			snaps.MatchSnapshot(GinkgoT(), output.Traffic.RateLimit)
		})

		It("must handle empty rate limits", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				// RateLimit is initialized but empty
				RateLimit: api.RateLimitContainer{},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			// Traffic could be nil if no traffic settings were set
			if output.Traffic != nil {
				Expect(output.Traffic.RateLimit).To(BeNil())
			} else {
				// If Traffic is nil, that's fine too
				Expect(output.Traffic).To(BeNil())
			}
		})

		It("must handle nil circuit breaker", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}

			output := &roverv1.ApiExposure{}

			// When
			mapCircuitBreaker(input, output)

			// Then
			Expect(output.Traffic).To(BeNil())
		})

		It("must handle configured circuit breaker", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
				CircuitBreaker: api.CircuitBreaker{
					Enabled: true,
				},
			}

			output := &roverv1.ApiExposure{}

			// When
			mapCircuitBreaker(input, output)

			// Then
			Expect(output.Traffic.CircuitBreaker).NotTo(BeNil())
			Expect(output.Traffic.CircuitBreaker.Enabled).To(BeTrue())
		})
	})
})
