// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("File Type (SFTP) Exposure Mapper", func() {

	Context("mapFileExposure", func() {
		It("must map a FileExposure correctly", func() {
			input := &roverv1.FileExposure{
				FileType:   "demo-sftp-spec-v1",
				Visibility: roverv1.VisibilityWorld,
				PublicKeys: []roverv1.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			}

			output := mapFileExposure(input)

			Expect(output.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(output.Visibility).To(Equal(api.WORLD))
			Expect(output.PublicKeys).To(HaveLen(1))
			Expect(output.PublicKeys[0].Label).To(Equal("provider-key"))
			Expect(output.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAA1"))
		})

		It("must map the approval strategy and trusted teams back to the API", func() {
			input := &roverv1.FileExposure{
				FileType:   "demo-sftp-spec-v1",
				Visibility: roverv1.VisibilityWorld,
				Approval: roverv1.Approval{
					Strategy:     roverv1.ApprovalStrategyFourEyes,
					TrustedTeams: []roverv1.TrustedTeam{{Group: "group", Team: "team"}},
				},
			}

			output := mapFileExposure(input)

			Expect(output.Approval).To(Equal(api.FOUREYES))
			Expect(output.TrustedTeams).To(HaveLen(1))
			Expect(output.TrustedTeams[0].Team).To(Equal("group--team"))
		})

		DescribeTable("must map visibility to the API visibility", func(in roverv1.Visibility, expected api.Visibility) {
			output := mapFileExposure(&roverv1.FileExposure{
				FileType:   "demo-sftp-spec-v1",
				Visibility: in,
				PublicKeys: []roverv1.PublicKey{{Label: "provider-key", Key: "ssh-ed25519 AAAA1"}},
			})
			Expect(output.Visibility).To(Equal(expected))
		},
			Entry("WORLD", roverv1.VisibilityWorld, api.WORLD),
			Entry("ZONE", roverv1.VisibilityZone, api.ZONE),
			Entry("ENTERPRISE", roverv1.VisibilityEnterprise, api.ENTERPRISE),
		)
	})

	Context("mapPublicKeys", func() {
		It("must return nil for an empty list", func() {
			Expect(mapPublicKeys(nil)).To(BeNil())
			Expect(mapPublicKeys([]roverv1.PublicKey{})).To(BeNil())
		})

		It("must preserve order and values", func() {
			output := mapPublicKeys([]roverv1.PublicKey{
				{Label: "a", Key: "k1"},
				{Label: "b", Key: "k2"},
			})

			Expect(output).To(HaveLen(2))
			Expect(output[0]).To(Equal(api.PublicKey{Label: "a", Key: "k1"}))
			Expect(output[1]).To(Equal(api.PublicKey{Label: "b", Key: "k2"}))
		})
	})

	Context("mapExposure dispatch", func() {
		It("must map a FileExposure via the discriminator", func() {
			input := &roverv1.Exposure{
				File: &roverv1.FileExposure{
					FileType:   "demo-sftp-spec-v1",
					Visibility: roverv1.VisibilityWorld,
					PublicKeys: []roverv1.PublicKey{
						{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
					},
				},
			}
			output := &api.Exposure{}

			err := mapExposure(input, output)
			Expect(err).To(BeNil())

			fileExposure, err := output.AsFileExposure()
			Expect(err).To(BeNil())
			Expect(fileExposure.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(fileExposure.Visibility).To(Equal(api.WORLD))
			Expect(fileExposure.PublicKeys).To(HaveLen(1))
		})
	})
})
