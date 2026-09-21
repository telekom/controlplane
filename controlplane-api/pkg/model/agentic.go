// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// AgenticExposureSecurity represents the security config for an AgenticExposure.
type AgenticExposureSecurity struct {
	M2M *Machine2MachineAuthentication `json:"m2m,omitempty"`
}

// AgenticSubscriptionSecurity represents the security config for an AgenticSubscription.
type AgenticSubscriptionSecurity struct {
	M2M *SubscriberMachine2MachineAuthentication `json:"m2m,omitempty"`
}

// AgenticSubscriberTraffic represents the traffic config for an AgenticSubscription.
// Unlike the exposure-side Traffic, subscriber failover here is a simple opt-in flag
// (eligible zones are derived automatically), not an explicit zone list.
type AgenticSubscriberTraffic struct {
	Failover *AgenticSubscriberFailover `json:"failover,omitempty"`
}

// AgenticSubscriberFailover defines the opt-in failover flag for an AgenticSubscription.
type AgenticSubscriberFailover struct {
	Enabled bool `json:"enabled"`
}

// AgenticTransformation defines request/response transformations for an
// AgenticExposure or AgenticSubscription.
type AgenticTransformation struct {
	Request AgenticRequestResponseTransformation `json:"request"`
}

// AgenticRequestResponseTransformation defines transformations applied to agentic requests/responses.
type AgenticRequestResponseTransformation struct {
	Headers AgenticHeaderTransformation `json:"headers"`
}

// AgenticHeaderTransformation defines HTTP header modifications for agentic routes.
type AgenticHeaderTransformation struct {
	Remove []string `json:"remove,omitempty"`
	Add    []string `json:"add,omitempty"`
}
