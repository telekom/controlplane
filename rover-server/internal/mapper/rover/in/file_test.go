// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("File Type (SFTP) Mapper", func() {

	Context("mapFileExposure", func() {
		It("must map a FileExposure correctly", func() {
			input := api.FileExposure{
				Type:       "file",
				FileType:   "demo-sftp-spec-v1",
				Visibility: api.WORLD,
				PublicKeys: []api.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			}

			output := mapFileExposure(input)

			Expect(output).ToNot(BeNil())
			Expect(output.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(output.Visibility).To(Equal(roverv1.VisibilityWorld))
			Expect(output.PublicKeys).To(HaveLen(1))
			Expect(output.PublicKeys[0].Label).To(Equal("provider-key"))
			Expect(output.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAA1"))
		})

		It("must map the approval strategy", func() {
			input := api.FileExposure{
				Type:       "file",
				FileType:   "demo-sftp-spec-v1",
				Visibility: api.WORLD,
				Approval:   api.AUTO,
			}

			output := mapFileExposure(input)

			Expect(output.Approval.Strategy).To(Equal(roverv1.ApprovalStrategyAuto))
		})

		It("must default the approval strategy to Simple when omitted", func() {
			input := api.FileExposure{
				Type:     "file",
				FileType: "demo-sftp-spec-v1",
			}

			output := mapFileExposure(input)

			Expect(output.Approval.Strategy).To(Equal(roverv1.ApprovalStrategySimple))
		})

		It("must map trusted teams", func() {
			input := api.FileExposure{
				Type:         "file",
				FileType:     "demo-sftp-spec-v1",
				Approval:     api.FOUREYES,
				TrustedTeams: []api.TrustedTeam{{Team: "group--team"}},
			}

			output := mapFileExposure(input)

			Expect(output.Approval.Strategy).To(Equal(roverv1.ApprovalStrategyFourEyes))
			Expect(output.Approval.TrustedTeams).To(HaveLen(1))
			Expect(output.Approval.TrustedTeams[0].Group).To(Equal("group"))
			Expect(output.Approval.TrustedTeams[0].Team).To(Equal("team"))
		})

		It("must default visibility to Enterprise when omitted", func() {
			// The mapper defaults visibility via the shared toRoverVisibility,
			// which maps an empty value to Enterprise — consistent with
			// mapApiExposure/mapEventExposure and the CRD's
			// +kubebuilder:default=Enterprise.
			input := api.FileExposure{
				Type:     "file",
				FileType: "demo-sftp-spec-v1",
				PublicKeys: []api.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			}

			output := mapFileExposure(input)

			Expect(output.Visibility).To(Equal(roverv1.VisibilityEnterprise))
		})

		It("must map ZONE visibility to the CRD Zone visibility", func() {
			input := api.FileExposure{
				Type:       "file",
				FileType:   "demo-sftp-spec-v1",
				Visibility: api.ZONE,
				PublicKeys: []api.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			}

			output := mapFileExposure(input)

			Expect(output.Visibility).To(Equal(roverv1.VisibilityZone))
		})

		It("must map ENTERPRISE visibility to the CRD Enterprise visibility", func() {
			input := api.FileExposure{
				Type:       "file",
				FileType:   "demo-sftp-spec-v1",
				Visibility: api.ENTERPRISE,
				PublicKeys: []api.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			}

			output := mapFileExposure(input)

			Expect(output.Visibility).To(Equal(roverv1.VisibilityEnterprise))
		})
	})

	Context("mapFileSubscription", func() {
		It("must map a FileSubscription correctly", func() {
			input := api.FileSubscription{
				Type:     "file",
				FileType: "demo-sftp-spec-v1",
				PublicKeys: []api.PublicKey{
					{Label: "consumer-key", Key: "ssh-ed25519 AAAA2"},
				},
			}

			output := mapFileSubscription(input)

			Expect(output).ToNot(BeNil())
			Expect(output.FileType).To(Equal("demo-sftp-spec-v1"))
			Expect(output.PublicKeys).To(HaveLen(1))
			Expect(output.PublicKeys[0].Label).To(Equal("consumer-key"))
			Expect(output.PublicKeys[0].Key).To(Equal("ssh-ed25519 AAAA2"))
		})
	})

	Context("mapPublicKeys", func() {
		It("must return nil for an empty list", func() {
			Expect(mapPublicKeys(nil)).To(BeNil())
			Expect(mapPublicKeys([]api.PublicKey{})).To(BeNil())
		})

		It("must preserve order and values", func() {
			output := mapPublicKeys([]api.PublicKey{
				{Label: "a", Key: "k1"},
				{Label: "b", Key: "k2"},
			})

			Expect(output).To(HaveLen(2))
			Expect(output[0]).To(Equal(roverv1.PublicKey{Label: "a", Key: "k1"}))
			Expect(output[1]).To(Equal(roverv1.PublicKey{Label: "b", Key: "k2"}))
		})
	})

	Context("mapExposure dispatch", func() {
		It("must map a FileExposure via the discriminator", func() {
			exposure := &api.Exposure{}
			Expect(exposure.FromFileExposure(api.FileExposure{
				Type:     "file",
				FileType: "demo-sftp-spec-v1",
				PublicKeys: []api.PublicKey{
					{Label: "provider-key", Key: "ssh-ed25519 AAAA1"},
				},
			})).To(Succeed())

			output := &roverv1.Exposure{}
			err := mapExposure(exposure, output)

			Expect(err).To(BeNil())
			Expect(output.File).ToNot(BeNil())
			Expect(output.Api).To(BeNil())
			Expect(output.Event).To(BeNil())
			Expect(output.File.FileType).To(Equal("demo-sftp-spec-v1"))
		})
	})

	Context("mapSubscription dispatch", func() {
		It("must map a FileSubscription via the discriminator", func() {
			subscription := &api.Subscription{}
			Expect(subscription.FromFileSubscription(api.FileSubscription{
				Type:     "file",
				FileType: "demo-sftp-spec-v1",
				PublicKeys: []api.PublicKey{
					{Label: "consumer-key", Key: "ssh-ed25519 AAAA2"},
				},
			})).To(Succeed())

			output := &roverv1.Subscription{}
			err := mapSubscription(subscription, output)

			Expect(err).To(BeNil())
			Expect(output.File).ToNot(BeNil())
			Expect(output.Api).To(BeNil())
			Expect(output.Event).To(BeNil())
			Expect(output.File.FileType).To(Equal("demo-sftp-spec-v1"))
		})
	})
})
