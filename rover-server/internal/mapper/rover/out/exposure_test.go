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

var _ = Describe("Exposure Mapper", func() {
	Context("mapApiExposure", func() {
		It("must map ApiExposure correctly", func() {
			input := &apiExposure

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiExposure input", func() {
			input := &roverv1.ApiExposure{}

			output := mapApiExposure(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapExposure", func() {
		It("must map an ApiExposure correctly", func() {
			input := GetApiExposure(&apiExposure)
			output := &api.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventExposure correctly", func() {
			input := GetEventExposure(&eventExposure)
			output := &api.Exposure{}

			err := mapExposure(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error for unknown exposure type", func() {
			input := &roverv1.Exposure{}
			output := &api.Exposure{}

			err := mapExposure(input, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("unknown exposure type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("toApiVisibility", func() {
		It("must map WORLD visibility correctly", func() {
			input := roverv1.VisibilityWorld

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.WORLD))
		})

		It("must map ZONE visibility correctly", func() {
			input := roverv1.VisibilityZone

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.ZONE))
		})

		It("must map ENTERPRISE visibility correctly", func() {
			input := roverv1.VisibilityEnterprise

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.ENTERPRISE))
		})

		It("must map unknown visibility", func() {
			input := roverv1.Visibility("Unknown")

			output := toApiVisibility(input)

			Expect(output).To(Equal(api.Visibility("UNKNOWN")))
		})

		Context("toApiApprovalStrategy", func() {
			It("must map AUTO approval strategy correctly", func() {
				input := roverv1.ApprovalStrategyAuto

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.AUTO))
			})

			It("must map MANUAL approval strategy correctly", func() {
				input := roverv1.ApprovalStrategySimple

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.SIMPLE))
			})

			It("must map FOUREYES approval strategy correctly", func() {
				input := roverv1.ApprovalStrategyFourEyes

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.FOUREYES))
			})

			It("must map unknown approval strategy", func() {
				input := roverv1.ApprovalStrategy("Unknown")

				output := toApiApprovalStrategy(input)

				Expect(output).To(Equal(api.ApprovalStrategy("UNKNOWN")))
			})
		})
	})
})
