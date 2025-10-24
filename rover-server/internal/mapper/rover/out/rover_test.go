// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Rover Mapper", func() {
	Context("MapRover", func() {
		It("must map the internal format to the api format", func() {
			input := rover
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output := &api.Rover{}

			err := MapRover(nil, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

	})

	Context("MapExposures", func() {
		It("must map exposures correctly", func() {
			input := rover
			output := &api.Rover{}

			err := mapExposures(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("MapSubscriptions", func() {
		It("must map subscriptions correctly", func() {
			input := rover
			output := &api.Rover{}

			err := mapSubscriptions(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("MapRoverResponse", func() {
		It("must map a Rover to a RoverResponse correctly", func() {
			input := GetRoverWithReadyCondition(rover)

			output, err := MapRoverResponse(ctx, input)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapRoverResponse(ctx, nil)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

	})
})
