// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
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

	Context("MapAuthentication", func() {
		It("must map header from CRD to BASIC in API", func() {
			input := rover.DeepCopy()
			input.Spec.Authentication = &roverv1.RoverAuthentication{
				M2M: &roverv1.RoverM2MAuthentication{
					TokenRequest: "header",
				},
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Authentication.ClientAuthMethod).To(Equal(api.BASIC))
		})

		It("must map body from CRD to POST in API", func() {
			input := rover.DeepCopy()
			input.Spec.Authentication = &roverv1.RoverAuthentication{
				M2M: &roverv1.RoverM2MAuthentication{
					TokenRequest: "body",
				},
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Authentication.ClientAuthMethod).To(Equal(api.POST))
		})

		It("must not set authentication when it is nil", func() {
			input := rover.DeepCopy()
			input.Spec.Authentication = nil
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Authentication).To(Equal(api.Authentication{}))
		})
	})

	Context("MapRoverResponse", func() {
		It("must map a Rover to a RoverResponse correctly", func() {
			input := GetRoverWithReadyCondition(rover)

			output, err := MapResponse(ctx, input, stores)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapResponse(ctx, nil, stores)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

	})
})
