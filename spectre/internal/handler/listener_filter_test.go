// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

const (
	testApiBasePath = "/api/v1/orders"
	testConsumerId  = "team-alpha--consumer-app"
	testProviderId  = "team-beta--provider-app"
)

// fixedAttrs returns the four mandatory selection attributes that every
// Trigger produced by buildBridgeTrigger must contain.
func fixedAttrs(kindValue string) map[string]string {
	return map[string]string{
		"issue":    testApiBasePath,
		"consumer": testConsumerId,
		"provider": testProviderId,
		"kind":     kindValue,
	}
}

var _ = Describe("buildBridgeTrigger", func() {

	// --- nil / empty filter ---

	It("should produce only fixed attributes when filter is nil", func() {
		trigger := buildBridgeTrigger(nil, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.SelectionFilter).ToNot(BeNil())
		Expect(trigger.SelectionFilter.Attributes).To(Equal(fixedAttrs("REQUEST")))
		Expect(trigger.ResponseFilter).To(BeNil())
	})

	It("should produce only fixed attributes when filter has nil maps", func() {
		filter := &spectrev1.ListenerFilter{Trigger: nil, Payload: nil}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.SelectionFilter.Attributes).To(Equal(fixedAttrs("REQUEST")))
		Expect(trigger.ResponseFilter).To(BeNil())
	})

	It("should produce only fixed attributes when trigger map and payload slice are empty", func() {
		filter := &spectrev1.ListenerFilter{
			Trigger: map[string]string{},
			Payload: []string{},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.SelectionFilter.Attributes).To(Equal(fixedAttrs("REQUEST")))
		Expect(trigger.ResponseFilter).To(BeNil())
	})

	// --- trigger entries merge ---

	It("should merge user trigger entries with fixed attributes", func() {
		filter := &spectrev1.ListenerFilter{
			Trigger: map[string]string{
				"region": "eu-west",
				"tier":   "gold",
			},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		attrs := trigger.SelectionFilter.Attributes
		Expect(attrs).To(HaveLen(6))
		Expect(attrs).To(HaveKeyWithValue("region", "eu-west"))
		Expect(attrs).To(HaveKeyWithValue("tier", "gold"))
		// fixed attributes still present
		Expect(attrs).To(HaveKeyWithValue("issue", testApiBasePath))
		Expect(attrs).To(HaveKeyWithValue("consumer", testConsumerId))
		Expect(attrs).To(HaveKeyWithValue("provider", testProviderId))
		Expect(attrs).To(HaveKeyWithValue("kind", "REQUEST"))
	})

	// --- fixed-key collision tests ---

	DescribeTable("should let fixed attribute win when user trigger key collides",
		func(key, userVal string) {
			filter := &spectrev1.ListenerFilter{
				Trigger: map[string]string{key: userVal},
			}
			trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
			expected := fixedAttrs("REQUEST")
			Expect(trigger.SelectionFilter.Attributes).To(HaveKeyWithValue(key, expected[key]))
			Expect(trigger.SelectionFilter.Attributes).To(HaveLen(4))
		},
		Entry("issue collides", "issue", "attacker-value"),
		Entry("consumer collides", "consumer", "attacker-value"),
		Entry("provider collides", "provider", "attacker-value"),
		Entry("kind collides", "kind", "attacker-value"),
	)

	// --- trigger values have no prefix ---

	It("should not add payload prefix to trigger values", func() {
		filter := &spectrev1.ListenerFilter{
			Trigger: map[string]string{"status": "active"},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("status", "active"))
	})

	// --- payload entries ---

	It("should prefix payload entries with 'payload.'", func() {
		filter := &spectrev1.ListenerFilter{
			Payload: []string{"$.data.name", "$.meta.id"},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.ResponseFilter).ToNot(BeNil())
		Expect(trigger.ResponseFilter.Paths).To(Equal([]string{
			"payload.$.data.name",
			"payload.$.meta.id",
		}))
	})

	It("should set ResponseFilter mode to Include", func() {
		filter := &spectrev1.ListenerFilter{
			Payload: []string{"$.data"},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.ResponseFilter.Mode).To(Equal(pubsubv1.ResponseFilterModeInclude))
	})

	// --- both trigger and payload ---

	It("should handle both trigger and payload set simultaneously", func() {
		filter := &spectrev1.ListenerFilter{
			Trigger: map[string]string{"region": "eu-west"},
			Payload: []string{"$.data"},
		}
		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")

		Expect(trigger.SelectionFilter.Attributes).To(HaveLen(5))
		Expect(trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("region", "eu-west"))

		Expect(trigger.ResponseFilter).ToNot(BeNil())
		Expect(trigger.ResponseFilter.Paths).To(Equal([]string{"payload.$.data"}))
		Expect(trigger.ResponseFilter.Mode).To(Equal(pubsubv1.ResponseFilterModeInclude))
	})

	// --- input not mutated ---

	It("should not mutate the input trigger map", func() {
		inputTrigger := map[string]string{"region": "eu-west"}
		filter := &spectrev1.ListenerFilter{Trigger: inputTrigger}

		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")

		// Mutate the returned attrs
		trigger.SelectionFilter.Attributes["injected"] = "evil"

		Expect(inputTrigger).To(HaveLen(1))
		Expect(inputTrigger).To(Equal(map[string]string{"region": "eu-west"}))
	})

	It("should not mutate the input payload slice", func() {
		inputPayload := []string{"$.data"}
		filter := &spectrev1.ListenerFilter{Payload: inputPayload}

		trigger := buildBridgeTrigger(filter, testApiBasePath, testConsumerId, testProviderId, "REQUEST")

		// Mutate the returned paths
		trigger.ResponseFilter.Paths[0] = "corrupted"

		Expect(inputPayload).To(Equal([]string{"$.data"}))
	})

	// --- kindValue variants ---

	It("should use REQUEST kindValue for request filter subscriber", func() {
		trigger := buildBridgeTrigger(nil, testApiBasePath, testConsumerId, testProviderId, "REQUEST")
		Expect(trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("kind", "REQUEST"))
	})

	It("should use RESPONSE kindValue for response filter subscriber", func() {
		trigger := buildBridgeTrigger(nil, testApiBasePath, testConsumerId, testProviderId, "RESPONSE")
		Expect(trigger.SelectionFilter.Attributes).To(HaveKeyWithValue("kind", "RESPONSE"))
	})
})
