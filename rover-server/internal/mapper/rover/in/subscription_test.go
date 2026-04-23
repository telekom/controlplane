// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription Mapper", func() {
	Context("MapApiSubscription", func() {
		It("must map ApiSubscription correctly", func() {
			input := apiSubscription

			output := mapApiSubscription(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must handle empty ApiSubscription input", func() {
			input := api.ApiSubscription{}

			output := mapApiSubscription(input)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapSubscription", func() {
		It("must map an ApiSubscription correctly", func() {
			input := GetApiSubscription(apiSubscription)
			output := &roverv1.Subscription{}

			err := mapSubscription(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must map an EventSubscription correctly", func() {
			input := GetEventSubscription(eventSubscription)
			output := &roverv1.Subscription{}

			err := mapSubscription(&input, output)

			Expect(err).ToNot(HaveOccurred())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if Discriminator fails", func() {
			input := &api.Subscription{}
			output := &roverv1.Subscription{}

			err := mapSubscription(input, output)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get subscription type"))
			snaps.MatchSnapshot(GinkgoT(), output)
		})
	})

	Context("mapEventSubscription", func() {
		It("must map delivery callback", func() {
			input := api.EventSubscription{
				EventType:    "de.telekom.test.v1",
				DeliveryType: "Callback",
				PayloadType:  "Data",
				Callback:     "https://callback.example.com/events",
			}

			output := mapEventSubscription(input)

			Expect(output.EventType).To(Equal("de.telekom.test.v1"))
			Expect(output.Delivery.Type).To(Equal(roverv1.EventDeliveryTypeCallback))
			Expect(output.Delivery.Payload).To(Equal(roverv1.EventPayloadTypeData))
			Expect(output.Delivery.Callback).To(Equal("https://callback.example.com/events"))
		})

		It("must map event retention time", func() {
			input := api.EventSubscription{
				EventType:          "de.telekom.test.v1",
				DeliveryType:       "ServerSentEvent",
				PayloadType:        "Data",
				EventRetentionTime: "P7D",
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.Type).To(Equal(roverv1.EventDeliveryTypeServerSentEvent))
			Expect(output.Delivery.EventRetentionTime).To(Equal("P7D"))
		})

		It("must map circuit breaker opt-out", func() {
			input := api.EventSubscription{
				EventType:            "de.telekom.test.v1",
				DeliveryType:         "Callback",
				PayloadType:          "Data",
				CircuitBreakerOptOut: true,
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.CircuitBreakerOptOut).To(BeTrue())
		})

		It("must map retryable status codes", func() {
			input := api.EventSubscription{
				EventType:            "de.telekom.test.v1",
				DeliveryType:         "Callback",
				PayloadType:          "Data",
				RetryableStatusCodes: []int{502, 503, 504},
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.RetryableStatusCodes).To(ConsistOf(502, 503, 504))
		})

		It("must map redeliveries per second", func() {
			input := api.EventSubscription{
				EventType:             "de.telekom.test.v1",
				DeliveryType:          "Callback",
				PayloadType:           "Data",
				RedeliveriesPerSecond: 5,
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.RedeliveriesPerSecond).ToNot(BeNil())
			Expect(*output.Delivery.RedeliveriesPerSecond).To(Equal(5))
		})

		It("must not set redeliveries per second when zero", func() {
			input := api.EventSubscription{
				EventType:             "de.telekom.test.v1",
				DeliveryType:          "Callback",
				PayloadType:           "Data",
				RedeliveriesPerSecond: 0,
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.RedeliveriesPerSecond).To(BeNil())
		})

		It("must map enforce GET health check", func() {
			input := api.EventSubscription{
				EventType:    "de.telekom.test.v1",
				DeliveryType: "Callback",
				PayloadType:  "Data",
				EnforceGetHttpRequestMethodForHealthCheck: true,
			}

			output := mapEventSubscription(input)

			Expect(output.Delivery.EnforceGetHttpRequestMethodForHealthCheck).To(BeTrue())
		})

		It("must map trigger with response filter", func() {
			input := api.EventSubscription{
				EventType:    "de.telekom.test.v1",
				DeliveryType: "Callback",
				PayloadType:  "Data",
				Trigger: api.EventTrigger{
					ResponseFilter:     []string{"$.data.id"},
					ResponseFilterMode: "Include",
				},
			}

			output := mapEventSubscription(input)

			Expect(output.Trigger).ToNot(BeNil())
			Expect(output.Trigger.ResponseFilter).ToNot(BeNil())
			Expect(output.Trigger.ResponseFilter.Paths).To(ConsistOf("$.data.id"))
			Expect(output.Trigger.ResponseFilter.Mode).To(Equal(roverv1.EventResponseFilterMode("Include")))
		})

		It("must map scopes", func() {
			input := api.EventSubscription{
				EventType:    "de.telekom.test.v1",
				DeliveryType: "Callback",
				PayloadType:  "Data",
				Scopes:       []string{"scope1", "scope2"},
			}

			output := mapEventSubscription(input)

			Expect(output.Scopes).To(ConsistOf("scope1", "scope2"))
		})
	})

	Context("mapEventTriggerForSubscription", func() {
		It("must map response filter", func() {
			input := api.EventTrigger{
				ResponseFilter:     []string{"$.data.id", "$.data.name"},
				ResponseFilterMode: "Exclude",
			}

			output := mapEventTriggerForSubscription(input)

			Expect(output).ToNot(BeNil())
			Expect(output.ResponseFilter).ToNot(BeNil())
			Expect(output.ResponseFilter.Paths).To(ConsistOf("$.data.id", "$.data.name"))
			Expect(output.ResponseFilter.Mode).To(Equal(roverv1.EventResponseFilterMode("Exclude")))
		})

		It("must map selection filter with attributes", func() {
			input := api.EventTrigger{
				SelectionFilter: map[string]string{"source": "my-app", "type": "create"},
			}

			output := mapEventTriggerForSubscription(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("type", "create"))
		})

		It("must map advanced selection filter", func() {
			input := api.EventTrigger{
				AdvancedSelectionFilter: map[string]any{"op": "eq", "field": "type", "value": "create"},
			}

			output := mapEventTriggerForSubscription(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Expression).ToNot(BeNil())
			Expect(output.SelectionFilter.Expression.Raw).ToNot(BeNil())
		})

		It("must map both attributes and advanced selection filter", func() {
			input := api.EventTrigger{
				SelectionFilter:         map[string]string{"source": "my-app"},
				AdvancedSelectionFilter: map[string]any{"op": "and", "children": []any{}},
			}

			output := mapEventTriggerForSubscription(input)

			Expect(output).ToNot(BeNil())
			Expect(output.SelectionFilter).ToNot(BeNil())
			Expect(output.SelectionFilter.Attributes).To(HaveKeyWithValue("source", "my-app"))
			Expect(output.SelectionFilter.Expression).ToNot(BeNil())
		})
	})

	Context("mapSubscriptionTraffic", func() {
		It("must map failover zones", func() {
			input := api.ApiSubscription{
				BasePath: "/test",
				Failover: api.Failover{
					Zones: []string{"zone-a", "zone-b"},
				},
			}

			output := &roverv1.ApiSubscription{}
			mapSubscriptionTraffic(input, output)

			Expect(output.Traffic.Failover).ToNot(BeNil())
			Expect(output.Traffic.Failover.Zones).To(ConsistOf("zone-a", "zone-b"))
		})

		It("must handle empty failover zones", func() {
			input := api.ApiSubscription{
				BasePath: "/test",
				Failover: api.Failover{
					Zones: []string{},
				},
			}

			output := &roverv1.ApiSubscription{}
			mapSubscriptionTraffic(input, output)

			Expect(output.Traffic.Failover).To(BeNil())
		})

		It("must handle nil failover", func() {
			input := api.ApiSubscription{
				BasePath: "/test",
			}

			output := &roverv1.ApiSubscription{}
			mapSubscriptionTraffic(input, output)

			Expect(output.Traffic.Failover).To(BeNil())
		})
	})
})
