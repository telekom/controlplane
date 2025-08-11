// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Trusted Teams Mapper", func() {
	Context("mapTrustedTeams", func() {
		It("must map trusted teams correctly", func() {
			input := &roverv1.ApiExposure{
				Approval: roverv1.Approval{
					TrustedTeams: []roverv1.TrustedTeam{
						{
							Group: "group1",
							Team:  "team1",
						},
						{
							Group: "group2",
							Team:  "team2",
						},
					},
				},
			}
			output := &api.ApiExposure{}

			mapTrustedTeams(input, output)

			Expect(output.TrustedTeams).To(HaveLen(2))
			Expect(output.TrustedTeams[0].Team).To(Equal("group1--team1"))
			Expect(output.TrustedTeams[1].Team).To(Equal("group2--team2"))
		})

		It("must handle empty trusted teams correctly", func() {
			input := &roverv1.ApiExposure{
				Approval: roverv1.Approval{
					TrustedTeams: []roverv1.TrustedTeam{},
				},
			}
			output := &api.ApiExposure{}

			mapTrustedTeams(input, output)

			Expect(output.TrustedTeams).To(HaveLen(0))
		})

		It("must handle nil trusted teams correctly", func() {
			input := &roverv1.ApiExposure{
				Approval: roverv1.Approval{
					TrustedTeams: nil,
				},
			}
			output := &api.ApiExposure{}

			mapTrustedTeams(input, output)

			Expect(output.TrustedTeams).To(BeNil())
		})
	})
})
