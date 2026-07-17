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

var _ = Describe("Listener Mapper In", func() {
	Context("mapListenersIn", func() {
		It("must not set listeners when input has none", func() {
			input := &api.Rover{Zone: "aws"}
			output := &roverv1.Rover{}

			MapRover(input, output)

			Expect(output.Spec.Listeners).To(BeNil())
		})

		It("must map listeners with all fields", func() {
			input := &api.Rover{
				Zone: "aws",
				Listeners: []api.RoverListener{
					{
						Consumer:    "eni--team--consumer",
						Provider:    "eni--team--provider",
						ApiBasePath: "/echo/v1",
						RequestFilter: api.ListenerFilter{
							Trigger: map[string]string{"method": "GET"},
							Payload: []string{"name", "id"},
						},
						ResponseFilter: api.ListenerFilter{
							Payload: []string{"status"},
						},
					},
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Listeners).To(HaveLen(1))
			Expect(output.Spec.Listeners[0].Consumer).To(Equal("eni--team--consumer"))
			Expect(output.Spec.Listeners[0].Provider).To(Equal("eni--team--provider"))
			Expect(output.Spec.Listeners[0].ApiBasePath).To(Equal("/echo/v1"))
			Expect(output.Spec.Listeners[0].RequestFilter).ToNot(BeNil())
			Expect(output.Spec.Listeners[0].RequestFilter.Trigger).To(HaveKeyWithValue("method", "GET"))
			Expect(output.Spec.Listeners[0].RequestFilter.Payload).To(ConsistOf("name", "id"))
			Expect(output.Spec.Listeners[0].ResponseFilter).ToNot(BeNil())
			Expect(output.Spec.Listeners[0].ResponseFilter.Payload).To(ConsistOf("status"))
		})

		It("must map listener with eventType and eventFilter", func() {
			input := &api.Rover{
				Zone: "aws",
				Listeners: []api.RoverListener{
					{
						Consumer:  "eni--team--consumer",
						Provider:  "eni--team--provider",
						EventType: "de.telekom.eni.my-event.v1",
						EventFilter: api.ListenerFilter{
							Payload: []string{"data"},
						},
					},
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Listeners).To(HaveLen(1))
			Expect(output.Spec.Listeners[0].EventType).To(Equal("de.telekom.eni.my-event.v1"))
			Expect(output.Spec.Listeners[0].EventFilter).ToNot(BeNil())
			Expect(output.Spec.Listeners[0].EventFilter.Payload).To(ConsistOf("data"))
		})

		It("must not set filter when empty", func() {
			input := &api.Rover{
				Zone: "aws",
				Listeners: []api.RoverListener{
					{
						Consumer: "eni--team--consumer",
						Provider: "eni--team--provider",
					},
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.Listeners[0].RequestFilter).To(BeNil())
			Expect(output.Spec.Listeners[0].ResponseFilter).To(BeNil())
			Expect(output.Spec.Listeners[0].EventFilter).To(BeNil())
		})
	})

	Context("mapListenerSubscriptionIn", func() {
		It("must not set listenerSubscription when input has none", func() {
			input := &api.Rover{Zone: "aws"}
			output := &roverv1.Rover{}

			MapRover(input, output)

			Expect(output.Spec.ListenerSubscription).To(BeNil())
		})

		It("must map listenerSubscription with callback delivery", func() {
			input := &api.Rover{
				Zone: "aws",
				ListenerSubscription: api.ListenerSubscription{
					DeliveryType: "callback",
					Callback:     "https://my-listener.example.com/events",
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.ListenerSubscription).ToNot(BeNil())
			Expect(output.Spec.ListenerSubscription.DeliveryType).To(Equal("callback"))
			Expect(output.Spec.ListenerSubscription.Callback).To(Equal("https://my-listener.example.com/events"))
		})

		It("must map listenerSubscription with server_sent_event delivery", func() {
			input := &api.Rover{
				Zone: "aws",
				ListenerSubscription: api.ListenerSubscription{
					DeliveryType: "server_sent_event",
				},
			}
			output := &roverv1.Rover{}

			err := MapRover(input, output)

			Expect(err).ToNot(HaveOccurred())
			Expect(output.Spec.ListenerSubscription).ToNot(BeNil())
			Expect(output.Spec.ListenerSubscription.DeliveryType).To(Equal("server_sent_event"))
			Expect(output.Spec.ListenerSubscription.Callback).To(BeEmpty())
		})
	})
})
