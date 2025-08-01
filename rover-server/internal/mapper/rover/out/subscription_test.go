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

var _ = Describe("Subscription Mapper", func() {
	Context("MapApiSubscription", func() {
		It("must map ApiSubscription correctly", func() {
			input := &apiSubscription

			output := mapApiSubscription(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiSubscription input", func() {
			input := &roverv1.ApiSubscription{}

			output := mapApiSubscription(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapSubscription", func() {
		It("must map an ApiSubscription correctly", func() {
			input := GetApiSubscription(&apiSubscription)
			output := &api.Subscription{}

			err := mapSubscription(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventSubscription correctly", func() {
			input := GetEventSubscription(&eventSubscription)
			output := &api.Subscription{}

			err := mapSubscription(&input, output)

			Expect(err).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if Discriminator fails", func() {
			input := &roverv1.Subscription{}
			output := &api.Subscription{}

			err := mapSubscription(input, output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("unknown subscription type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

})
