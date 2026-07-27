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
		It("must map client_secret_basic from CRD to BASIC in API", func() {
			input := rover.DeepCopy()
			input.Spec.Authentication = &roverv1.RoverAuthentication{
				M2M: &roverv1.RoverM2MAuthentication{
					TokenRequest: roverv1.TokenRequestClientSecretBasic,
				},
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Authentication.ClientAuthMethod).To(Equal(api.BASIC))
		})

		It("must map client_secret_post from CRD to POST in API", func() {
			input := rover.DeepCopy()
			input.Spec.Authentication = &roverv1.RoverAuthentication{
				M2M: &roverv1.RoverM2MAuthentication{
					TokenRequest: roverv1.TokenRequestClientSecretPost,
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

	Context("ExternalIds → scalars translation", func() {
		It("projects psi and icto schemes onto scalar response fields", func() {
			input := rover.DeepCopy()
			input.Spec.ExternalIds = append(input.Spec.ExternalIds,
				roverv1.ExternalId{Scheme: "psi", Id: "PSI-103596"},
				roverv1.ExternalId{Scheme: "icto", Id: "icto-12345"},
				roverv1.ExternalId{Scheme: "unknown", Id: "ignored"},
			)
			output := &api.Rover{}
			Expect(MapRover(input, output)).To(Succeed())
			Expect(output.Psiid).To(Equal("PSI-103596"))
			Expect(output.Icto).To(Equal("icto-12345"))
		})
	})

	Context("Consumer Failover", func() {
		It("must map per-subscription failover to global failover on Rover level", func() {
			input := rover.DeepCopy()
			By("setting a subscription with failover enabled")
			input.Spec.Zone = "test-zone"
			input.Spec.Subscriptions = []roverv1.Subscription{
				{
					Api: &roverv1.ApiSubscription{
						BasePath: "/eni/failover-test/v1",
						Traffic: roverv1.SubscriberTraffic{
							Failover: &roverv1.SubscriberFailover{
								Enabled: true,
							},
						},
					},
				},
			}

			By("mapping the rover")
			output := &api.Rover{}
			Expect(MapRover(input, output)).To(Succeed())

			By("checking that the failover is set on the rover level")
			Expect(output.FailoverEnabled).To(BeTrue())
		})
	})
})
