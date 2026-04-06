// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

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

	Context("mapEventSubscription", func() {
		It("must map delivery callback", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:     roverv1.EventDeliveryTypeCallback,
					Payload:  roverv1.EventPayloadTypeData,
					Callback: "https://callback.example.com/events",
				},
			}

			output := mapEventSubscription(input)

			Expect(output.EventType).To(Equal("de.telekom.test.v1"))
			Expect(output.DeliveryType).To(Equal("Callback"))
			Expect(output.PayloadType).To(Equal("Data"))
			Expect(output.Callback).To(Equal("https://callback.example.com/events"))
		})

		It("must map event retention time", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:               roverv1.EventDeliveryTypeServerSentEvent,
					Payload:            roverv1.EventPayloadTypeData,
					EventRetentionTime: "P7D",
				},
			}

			output := mapEventSubscription(input)

			Expect(output.DeliveryType).To(Equal("ServerSentEvent"))
			Expect(output.EventRetentionTime).To(Equal("P7D"))
		})

		It("must map circuit breaker opt-out", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:                 roverv1.EventDeliveryTypeCallback,
					Payload:              roverv1.EventPayloadTypeData,
					CircuitBreakerOptOut: true,
				},
			}

			output := mapEventSubscription(input)

			Expect(output.CircuitBreakerOptOut).To(BeTrue())
		})

		It("must map retryable status codes", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:                 roverv1.EventDeliveryTypeCallback,
					Payload:              roverv1.EventPayloadTypeData,
					RetryableStatusCodes: []int{502, 503, 504},
				},
			}

			output := mapEventSubscription(input)

			Expect(output.RetryableStatusCodes).To(ConsistOf(502, 503, 504))
		})

		It("must map redeliveries per second", func() {
			redeliveries := 5
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:                  roverv1.EventDeliveryTypeCallback,
					Payload:               roverv1.EventPayloadTypeData,
					RedeliveriesPerSecond: &redeliveries,
				},
			}

			output := mapEventSubscription(input)

			Expect(output.RedeliveriesPerSecond).To(Equal(5))
		})

		It("must map enforce GET health check", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:    roverv1.EventDeliveryTypeCallback,
					Payload: roverv1.EventPayloadTypeData,
					EnforceGetHttpRequestMethodForHealthCheck: true,
				},
			}

			output := mapEventSubscription(input)

			Expect(output.EnforceGetHttpRequestMethodForHealthCheck).To(BeTrue())
		})

		It("must map trigger", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:    roverv1.EventDeliveryTypeCallback,
					Payload: roverv1.EventPayloadTypeData,
				},
				Trigger: &roverv1.EventTrigger{
					ResponseFilter: &roverv1.EventResponseFilter{
						Paths: []string{"$.data.id"},
						Mode:  roverv1.EventResponseFilterModeInclude,
					},
				},
			}

			output := mapEventSubscription(input)

			Expect(output.Trigger.ResponseFilter).To(ConsistOf("$.data.id"))
			Expect(output.Trigger.ResponseFilterMode).To(Equal(api.EventTriggerResponseFilterMode("Include")))
		})

		It("must map scopes", func() {
			input := &roverv1.EventSubscription{
				EventType: "de.telekom.test.v1",
				Delivery: roverv1.EventDelivery{
					Type:    roverv1.EventDeliveryTypeCallback,
					Payload: roverv1.EventPayloadTypeData,
				},
				Scopes: []string{"scope1", "scope2"},
			}

			output := mapEventSubscription(input)

			Expect(output.Scopes).To(ConsistOf("scope1", "scope2"))
		})
	})

	Context("mapEventTriggerOutForSubscription", func() {
		It("must map response filter", func() {
			input := &roverv1.EventTrigger{
				ResponseFilter: &roverv1.EventResponseFilter{
					Paths: []string{"$.data.id", "$.data.name"},
					Mode:  roverv1.EventResponseFilterModeExclude,
				},
			}

			output := mapEventTriggerOutForSubscription(input)

			Expect(output.ResponseFilter).To(ConsistOf("$.data.id", "$.data.name"))
			Expect(output.ResponseFilterMode).To(Equal(api.EventTriggerResponseFilterMode("Exclude")))
		})

		It("must map selection filter with attributes", func() {
			input := &roverv1.EventTrigger{
				SelectionFilter: &roverv1.EventSelectionFilter{
					Attributes: map[string]string{"source": "my-app", "type": "create"},
				},
			}

			output := mapEventTriggerOutForSubscription(input)

			Expect(output.SelectionFilter).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.SelectionFilter).To(HaveKeyWithValue("type", "create"))
		})

		It("must map advanced selection filter with expression", func() {
			raw := []byte(`{"op":"eq","field":"type","value":"create"}`)
			input := &roverv1.EventTrigger{
				SelectionFilter: &roverv1.EventSelectionFilter{
					Expression: &apiextensionsv1.JSON{Raw: raw},
				},
			}

			output := mapEventTriggerOutForSubscription(input)

			Expect(output.AdvancedSelectionFilter).ToNot(BeNil())
			Expect(output.AdvancedSelectionFilter["op"]).To(Equal("eq"))
			Expect(output.AdvancedSelectionFilter["field"]).To(Equal("type"))
			Expect(output.AdvancedSelectionFilter["value"]).To(Equal("create"))
		})

		It("must map both attributes and expression", func() {
			raw := []byte(`{"op":"and","children":[]}`)
			input := &roverv1.EventTrigger{
				SelectionFilter: &roverv1.EventSelectionFilter{
					Attributes: map[string]string{"source": "my-app"},
					Expression: &apiextensionsv1.JSON{Raw: raw},
				},
			}

			output := mapEventTriggerOutForSubscription(input)

			Expect(output.SelectionFilter).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.AdvancedSelectionFilter).ToNot(BeNil())
			Expect(output.AdvancedSelectionFilter["op"]).To(Equal("and"))
		})
	})

	Context("mapSubscriptionTraffic", func() {
		// Failover tests removed: failover is now controlled at Rover level via RoverSpec.FailoverEnabled
		// The Rover CRD's SubscriberTraffic no longer has a Failover field
		// (Note: rover-server API still has the field for backward compatibility, but it's not mapped)
	})
})
