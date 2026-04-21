// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = DescribeTable("FuzzyMatchEventDeliveryType",
	func(input string, expected roverv1.EventDeliveryType) {
		Expect(FuzzyMatchEventDeliveryType(input)).To(Equal(expected))
	},
	// Callback variants
	Entry("callback", "callback", roverv1.EventDeliveryTypeCallback),
	Entry("call-back", "call-back", roverv1.EventDeliveryTypeCallback),
	Entry("call_back", "call_back", roverv1.EventDeliveryTypeCallback),
	Entry("callBack", "callBack", roverv1.EventDeliveryTypeCallback),
	Entry("Callback", "Callback", roverv1.EventDeliveryTypeCallback),
	// SSE variants
	Entry("sse", "sse", roverv1.EventDeliveryTypeServerSentEvent),
	Entry("server-sent-event", "server-sent-event", roverv1.EventDeliveryTypeServerSentEvent),
	Entry("server_sent_event", "server_sent_event", roverv1.EventDeliveryTypeServerSentEvent),
	Entry("ServerSentEvent", "ServerSentEvent", roverv1.EventDeliveryTypeServerSentEvent),
	// Default passthrough
	Entry("unknown passthrough", "webhook", roverv1.EventDeliveryType("webhook")),
	Entry("empty passthrough", "", roverv1.EventDeliveryType("")),
)

var _ = DescribeTable("FuzzyMatchEventPayloadType",
	func(input string, expected roverv1.EventPayloadType) {
		Expect(FuzzyMatchEventPayloadType(input)).To(Equal(expected))
	},
	// Data variants
	Entry("data", "data", roverv1.EventPayloadTypeData),
	Entry("Data", "Data", roverv1.EventPayloadTypeData),
	// DataRef variants
	Entry("data-ref", "data-ref", roverv1.EventPayloadTypeDataRef),
	Entry("dataref", "dataref", roverv1.EventPayloadTypeDataRef),
	Entry("data_ref", "data_ref", roverv1.EventPayloadTypeDataRef),
	Entry("DataRef", "DataRef", roverv1.EventPayloadTypeDataRef),
	// Default passthrough
	Entry("unknown passthrough", "binary", roverv1.EventPayloadType("binary")),
	Entry("empty passthrough", "", roverv1.EventPayloadType("")),
)

var _ = DescribeTable("FuzzyMatchEventResponseFilterMode",
	func(input string, expected roverv1.EventResponseFilterMode) {
		Expect(FuzzyMatchEventResponseFilterMode(input)).To(Equal(expected))
	},
	// Include variants
	Entry("include", "include", roverv1.EventResponseFilterModeInclude),
	Entry("INCLUDE", "INCLUDE", roverv1.EventResponseFilterModeInclude),
	Entry("Include", "Include", roverv1.EventResponseFilterModeInclude),
	// Exclude variants
	Entry("exclude", "exclude", roverv1.EventResponseFilterModeExclude),
	Entry("EXCLUDE", "EXCLUDE", roverv1.EventResponseFilterModeExclude),
	Entry("Exclude", "Exclude", roverv1.EventResponseFilterModeExclude),
	// Default passthrough
	Entry("unknown passthrough", "filter", roverv1.EventResponseFilterMode("filter")),
	Entry("empty passthrough", "", roverv1.EventResponseFilterMode("")),
)
