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
	})
})
