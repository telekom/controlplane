// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"strings"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// FuzzyMatchEventDeliveryType performs a fuzzy match on the input string to determine the EventDeliveryType.
func FuzzyMatchEventDeliveryType(in string) roverv1.EventDeliveryType {
	switch in {
	case "callback", "call-back", "call_back", "callBack", "Callback":
		return roverv1.EventDeliveryTypeCallback
	case "sse", "server-sent-event", "server_sent_event", "ServerSentEvent":
		return roverv1.EventDeliveryTypeServerSentEvent
	default:
		return roverv1.EventDeliveryType(in)
	}
}

// FuzzyMatchEventPayloadType performs a fuzzy match on the input string to determine the EventPayloadType.
func FuzzyMatchEventPayloadType(in string) roverv1.EventPayloadType {
	switch in {
	case "data", "Data":
		return roverv1.EventPayloadTypeData
	case "data-ref", "dataref", "data_ref", "DataRef":
		return roverv1.EventPayloadTypeDataRef
	default:
		return roverv1.EventPayloadType(in)
	}
}

// FuzzyMatchEventResponseFilterMode performs a fuzzy match on the input string to determine the EventResponseFilterMode.
func FuzzyMatchEventResponseFilterMode(in string) roverv1.EventResponseFilterMode {
	switch in {
	case "include", "INCLUDE", "Include":
		return roverv1.EventResponseFilterModeInclude
	case "exclude", "EXCLUDE", "Exclude":
		return roverv1.EventResponseFilterModeExclude
	default:
		return roverv1.EventResponseFilterMode(in)
	}
}

// FuzzyMatchClientAuthMethod performs a fuzzy match on the input string to determine the AuthenticationClientAuthMethod.
func FuzzyMatchClientAuthMethod(in string) api.AuthenticationClientAuthMethod {
	switch strings.ToLower(in) {
	case "basic":
		return api.BASIC
	case "body", "post":
		return api.POST
	case "none":
		return api.NONE
	default:
		return api.AuthenticationClientAuthMethod(in)
	}
}
