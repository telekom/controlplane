// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Trusted Teams Mapper", func() {
	Context("mapTrustedTeams", func() {
		It("must map trusted teams correctly", func() {
			input := api.ApiExposure{
				TrustedTeams: []api.TrustedTeam{
					{
						Team: "group1--team1",
					},
					{
						Team: "group2--team2",
					},
				},
			}
			output := &roverv1.ApiExposure{
				Approval: roverv1.Approval{},
			}

			mapTrustedTeams(input, output)

			Expect(output.Approval.TrustedTeams).To(HaveLen(2))
			Expect(output.Approval.TrustedTeams[0].Group).To(Equal("group1"))
			Expect(output.Approval.TrustedTeams[0].Team).To(Equal("team1"))
			Expect(output.Approval.TrustedTeams[1].Group).To(Equal("group2"))
			Expect(output.Approval.TrustedTeams[1].Team).To(Equal("team2"))
		})

		It("must handle empty trusted teams correctly", func() {
			input := api.ApiExposure{
				TrustedTeams: []api.TrustedTeam{},
			}
			output := &roverv1.ApiExposure{
				Approval: roverv1.Approval{},
			}

			mapTrustedTeams(input, output)

			Expect(output.Approval.TrustedTeams).To(HaveLen(0))
		})

		It("must handle nil trusted teams correctly", func() {
			input := api.ApiExposure{
				TrustedTeams: nil,
			}
			output := &roverv1.ApiExposure{
				Approval: roverv1.Approval{},
			}

			mapTrustedTeams(input, output)

			Expect(output.Approval.TrustedTeams).To(BeNil())
		})

	})
})
