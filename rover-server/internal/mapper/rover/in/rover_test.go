// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("Rover Mapper", func() {
	Context("MapRover", func() {
		It("must map the api format to the internal format", func() {
			input := apiRover
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output := &roverv1.Rover{}

			err := MapRover(nil, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

	})

	Context("MapExposures", func() {
		It("must map exposures correctly", func() {
			input := apiRover
			output := &roverv1.Rover{}

			err := mapExposures(input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("MapSubscriptions", func() {
		It("must map subscriptions correctly", func() {
			input := apiRover
			out := &roverv1.Rover{}

			err := mapSubscriptions(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})
	})

	Context("MapAuthorization", func() {
		It("must map authorization in flat format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Resource: "stargate:payment:v1",
					Role:     "admin",
					Actions:  []string{"read", "write"},
				},
			}
			out := &roverv1.Rover{}

			err := mapAuthorization(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must map authorization in resource-oriented format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Resource: "stargate:payment:v1",
					Permissions: []api.AuthorizationPermissionInfo{
						{
							Role:    "admin",
							Actions: []string{"read", "write", "delete"},
						},
						{
							Role:    "viewer",
							Actions: []string{"read"},
						},
					},
				},
			}
			out := &roverv1.Rover{}

			err := mapAuthorization(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must map authorization in role-oriented format", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{
				{
					Role: "admin",
					Permissions: []api.AuthorizationPermissionInfo{
						{
							Resource: "stargate:payment:v1",
							Actions:  []string{"read", "write"},
						},
						{
							Resource: "stargate:billing:v1",
							Actions:  []string{"read", "write"},
						},
					},
				},
			}
			out := &roverv1.Rover{}

			err := mapAuthorization(input, out)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), out)
		})

		It("must handle empty authorization", func() {
			input := apiRover
			input.Authorization = []api.AuthorizationInfo{}
			out := &roverv1.Rover{}

			err := mapAuthorization(input, out)

			Expect(err).To(BeNil())
			Expect(out.Spec.Authorization).To(BeEmpty())
		})
	})

	Context("MapRequest", func() {
		It("must map a RoverUpdateRequest to a Rover correctly", func() {
			output, err := MapRequest(roverUpdateRequest, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapRequest(nil, resourceIdInfo)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover update request is nil"))
		})

		It("must set the clientSecret if provided", func() {
			input := roverUpdateRequest
			input.ClientSecret = "supersecret"

			output, err := MapRequest(input, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			viper.Set("migration.active", true)

			output, err = MapRequest(input, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

	})
})
