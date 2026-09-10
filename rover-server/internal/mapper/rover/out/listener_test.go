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

var _ = Describe("Listener Mapper", func() {
	Context("mapListeners", func() {
		It("must not set listeners when input has none", func() {
			input := rover.DeepCopy()
			input.Spec.Listeners = nil
			output := &api.Rover{}

			MapRover(input, output)

			Expect(output.Listeners).To(BeNil())
		})

		It("must map listeners with all fields", func() {
			input := rover.DeepCopy()
			input.Spec.Listeners = []roverv1.RoverListener{
				{
					Consumer:    "eni--team--consumer",
					Provider:    "eni--team--provider",
					ApiBasePath: "/echo/v1",
				},
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Listeners).To(HaveLen(1))
			Expect(output.Listeners[0].Consumer).To(Equal("eni--team--consumer"))
			Expect(output.Listeners[0].Provider).To(Equal("eni--team--provider"))
			Expect(output.Listeners[0].ApiBasePath).To(Equal("/echo/v1"))
		})
	})

	Context("mapListenerSubscription", func() {
		It("must not set listenerSubscription when input has none", func() {
			input := rover.DeepCopy()
			input.Spec.ListenerSubscription = nil
			output := &api.Rover{}

			MapRover(input, output)

			Expect(output.ListenerSubscription).To(Equal(api.ListenerSubscription{}))
		})

		It("must map listenerSubscription with callback delivery", func() {
			input := rover.DeepCopy()
			input.Spec.ListenerSubscription = &roverv1.ListenerSubscription{
				DeliveryType: "callback",
				Callback:     "https://my-listener.example.com/events",
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.ListenerSubscription.DeliveryType).To(Equal(api.ListenerSubscriptionDeliveryType("callback")))
			Expect(output.ListenerSubscription.Callback).To(Equal("https://my-listener.example.com/events"))
		})

		It("must map listenerSubscription with server_sent_event delivery", func() {
			input := rover.DeepCopy()
			input.Spec.ListenerSubscription = &roverv1.ListenerSubscription{
				DeliveryType: "server_sent_event",
			}
			output := &api.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.ListenerSubscription.DeliveryType).To(Equal(api.ListenerSubscriptionDeliveryType("server_sent_event")))
			Expect(output.ListenerSubscription.Callback).To(BeEmpty())
		})
	})
})
