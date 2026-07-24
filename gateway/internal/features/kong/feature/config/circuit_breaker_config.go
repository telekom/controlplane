// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import kong "github.com/telekom/controlplane/gateway/pkg/kong/api"

type CircuitBreaker struct {
	Passive PassiveHealthcheck `json:"passive"`
	Active  ActiveHealthCheck  `json:"active"`
}

type PassiveHealthcheck struct {
	HealthyHttpStatuses   []int `json:"healthyHttpStatuses"`
	HealthySuccesses      int   `json:"healthySuccesses"`
	UnhealthyHttpFailures int   `json:"unhealthyHttpFailures"`
	UnhealthyHttpStatuses []int `json:"unhealthyHttpStatuses"`
	UnhealthyTcpFailures  int   `json:"unhealthyTcpFailures"`
	UnhealthyTimeouts     int   `json:"unhealthyTimeouts"`
}

type ActiveHealthCheck struct {
	HealthyHttpStatuses   []int `json:"healthyHttpStatuses"`
	UnhealthyHttpStatuses []int `json:"unhealthyHttpStatuses"`
}

var DefaultCircuitBreaker = &CircuitBreaker{
	Passive: PassiveHealthcheck{
		HealthyHttpStatuses:   []int{200, 201, 202, 203, 204, 205, 206, 207, 208, 226, 300, 301, 302, 303, 304, 305, 306, 307, 308},
		HealthySuccesses:      30,
		UnhealthyHttpFailures: 10,
		UnhealthyHttpStatuses: []int{429, 500, 503},
		UnhealthyTcpFailures:  10,
		UnhealthyTimeouts:     10,
	},
	Active: ActiveHealthCheck{
		HealthyHttpStatuses:   []int{302, 200},
		UnhealthyHttpStatuses: []int{429, 404, 500, 501, 502, 503, 504, 505},
	},
}

func ToPassiveUnhealthyHttpStatuses(statuses []int) *[]kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses {
	result := make([]kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses, len(statuses))
	for i, status := range statuses {
		result[i] = kong.CreateUpstreamRequestHealthchecksPassiveUnhealthyHttpStatuses(status)
	}
	return &result
}

func ToPassiveHealthyHttpStatuses(statuses []int) *[]kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses {
	result := make([]kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses, len(statuses))
	for i, status := range statuses {
		result[i] = kong.CreateUpstreamRequestHealthchecksPassiveHealthyHttpStatuses(status)
	}
	return &result
}
