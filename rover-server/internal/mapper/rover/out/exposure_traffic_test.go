// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

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
			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					Failover: &roverv1.Failover{
						Zones: []string{"zone1", "zone2"},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Failover.Zones).ToNot(BeNil())
			Expect(output.Failover.Zones).To(HaveLen(2))
			Expect(output.Failover.Zones).To(ContainElements("zone1", "zone2"))
			snaps.MatchSnapshot(GinkgoT(), output.Failover)
		})

		It("must handle nil failover zones", func() {
			// Given
			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					// Failover is not initialized
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.Failover.Zones).To(BeEmpty())
		})

		It("must map provider rate limits correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					RateLimit: &roverv1.RateLimit{
						Provider: &roverv1.RateLimitConfig{
							Limits: &roverv1.Limits{
								Second: 100,
								Minute: 1000,
								Hour:   10000,
							},
							Options: roverv1.RateLimitOptions{
								FaultTolerant:     true,
								HideClientHeaders: true,
							},
						},
					},
				},
			}

			output := &api.ApiExposure{
				// Initialize with empty rate limit container to match new code
				RateLimit: api.RateLimitContainer{},
			}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.RateLimit.Provider.Second).To(Equal(int32(100)))
			Expect(output.RateLimit.Provider.Minute).To(Equal(int32(1000)))
			Expect(output.RateLimit.Provider.Hour).To(Equal(int32(10000)))
			Expect(output.RateLimit.Provider.FaultTolerant).To(BeTrue())
			Expect(output.RateLimit.Provider.HideClientHeaders).To(BeTrue())
			snaps.MatchSnapshot(GinkgoT(), output.RateLimit)
		})

		It("must map consumer default rate limits correctly", func() {
			// Given

			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					RateLimit: &roverv1.RateLimit{
						Consumers: &roverv1.ConsumerRateLimits{
							Default: &roverv1.ConsumerRateLimitDefaults{
								Limits: roverv1.Limits{
									Second: 50,
									Minute: 500,
									Hour:   5000,
								},
							},
						},
					},
				},
			}

			output := &api.ApiExposure{
				// Initialize with empty rate limit container to match new code
				RateLimit: api.RateLimitContainer{},
			}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.RateLimit.ConsumerDefault.Second).To(Equal(int32(50)))
			Expect(output.RateLimit.ConsumerDefault.Minute).To(Equal(int32(500)))
			Expect(output.RateLimit.ConsumerDefault.Hour).To(Equal(int32(5000)))
			snaps.MatchSnapshot(GinkgoT(), output.RateLimit)
		})

		It("must map consumer overrides correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					RateLimit: &roverv1.RateLimit{
						Consumers: &roverv1.ConsumerRateLimits{
							Overrides: []roverv1.ConsumerRateLimitOverrides{
								{
									Consumer: "consumer1",
									Limits: roverv1.Limits{
										Second: 25,
										Minute: 250,
										Hour:   2500,
									},
								},
								{
									Consumer: "consumer2",
									Limits: roverv1.Limits{
										Second: 75,
										Minute: 750,
										Hour:   7500,
									},
								},
							},
						},
					},
				},
			}

			output := &api.ApiExposure{
				// Initialize with empty rate limit container to match new code
				RateLimit: api.RateLimitContainer{},
			}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.RateLimit.Consumers).To(HaveLen(2))

			// First consumer
			Expect(output.RateLimit.Consumers[0].Id).To(Equal("consumer1"))
			Expect(output.RateLimit.Consumers[0].Second).To(Equal(int32(25)))
			Expect(output.RateLimit.Consumers[0].Minute).To(Equal(int32(250)))
			Expect(output.RateLimit.Consumers[0].Hour).To(Equal(int32(2500)))

			// Second consumer
			Expect(output.RateLimit.Consumers[1].Id).To(Equal("consumer2"))
			Expect(output.RateLimit.Consumers[1].Second).To(Equal(int32(75)))
			Expect(output.RateLimit.Consumers[1].Minute).To(Equal(int32(750)))
			Expect(output.RateLimit.Consumers[1].Hour).To(Equal(int32(7500)))

			snaps.MatchSnapshot(GinkgoT(), output.RateLimit)
		})

		It("must handle empty rate limits", func() {
			// Given
			input := &roverv1.ApiExposure{
				Traffic: &roverv1.Traffic{
					// RateLimit is not initialized
				},
			}

			output := &api.ApiExposure{
				// Initialize with empty rate limit container to match new code
				RateLimit: api.RateLimitContainer{},
			}

			// When
			mapExposureTraffic(input, output)

			// Then
			Expect(output.RateLimit.Provider.Second).To(Equal(int32(0)))
			Expect(output.RateLimit.Provider.Minute).To(Equal(int32(0)))
			Expect(output.RateLimit.Provider.Hour).To(Equal(int32(0)))
			Expect(output.RateLimit.ConsumerDefault.Second).To(Equal(int32(0)))
			Expect(output.RateLimit.Consumers).To(BeEmpty())
		})
	})
})
