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

var _ = Describe("Exposure Mapper", func() {
	Context("mapApiExposure", func() {
		It("must map ApiExposure correctly", func() {
			input := apiExposure

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiExposure input", func() {
			input := api.ApiExposure{}

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapExposure", func() {
		It("must map an ApiExposure correctly", func() {
			input := GetApiExposure(apiExposure)
			output := &roverv1.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventExposure correctly", func() {
			input := GetEventExposure(eventExposure)
			output := &roverv1.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error for unknown exposure type", func() {
			input := &api.Exposure{}
			output := &roverv1.Exposure{}

			err := mapExposure(input, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to get exposure type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("toRoverVisibility", func() {
		It("must map WORLD visibility correctly", func() {
			input := api.WORLD

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityWorld))
		})

		It("must map ZONE visibility correctly", func() {
			input := api.ZONE

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityZone))
		})

		It("must map ENTERPRISE visibility correctly", func() {
			input := api.ENTERPRISE

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.VisibilityEnterprise))
		})

		It("must map unknown visibility", func() {
			input := api.Visibility("unknown")

			output := toRoverVisibility(input)

			Expect(output).To(Equal(roverv1.Visibility("Unknown")))
		})
	})

	Context("toRoverApprovalStrategy", func() {
		It("must map AUTO approval strategy correctly", func() {
			input := api.AUTO

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategyAuto))
		})

		It("must map MANUAL approval strategy correctly", func() {
			input := api.SIMPLE

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategySimple))
		})

		It("must map FOUREYES approval strategy correctly", func() {
			input := api.FOUREYES

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategyFourEyes))
		})

		It("must map unknown approval strategy", func() {
			input := api.ApprovalStrategy("unknown")

			output := toRoverApprovalStrategy(input)

			Expect(output).To(Equal(roverv1.ApprovalStrategy("Unknown")))
		})
	})
})
