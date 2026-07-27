// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"
)

func TestNamingConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "CallbackClientName",
			got:      CallbackClientName,
			expected: "eventstore",
		},
		{
			name:     "AdminClientName",
			got:      AdminClientName,
			expected: "horizon-quasar",
		},
		{
			name:     "CallbackURLQueryParam",
			got:      CallbackURLQueryParam,
			expected: "callback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.got)
			}
		})
	}
}

func TestNamingMakePublishRouteName(t *testing.T) {
	if got := makePublishRouteName(); got != "publish" {
		t.Errorf("makePublishRouteName() = %q, want %q", got, "publish")
	}
}

func TestNamingMakePublishRoutePath(t *testing.T) {
	if got := makePublishEventsRoutePath(); got != "/horizon/events/v1" {
		t.Errorf("makePublishEventsRoutePath() = %q, want %q", got, "/horizon/events/v1")
	}
	if got := makePublishRoutePath(); got != "/horizon/publish/v1" {
		t.Errorf("makePublishRoutePath() = %q, want %q", got, "/horizon/publish/v1")
	}
}

func TestNamingMakeSSERouteName(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{
			name:      "dotted event type",
			eventType: "de.telekom.test.v1",
			expected:  "sse--de-telekom-test-v1",
		},
		{
			name:      "already hyphenated",
			eventType: "simple-event",
			expected:  "sse--simple-event",
		},
		{
			name:      "single segment",
			eventType: "events",
			expected:  "sse--events",
		},
		{
			name:      "uppercase letters get lowered",
			eventType: "DE.Telekom.Test.V1",
			expected:  "sse--de-telekom-test-v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeSSERouteName(tc.eventType)
			if got != tc.expected {
				t.Errorf("makeSSERouteName(%q) = %q, want %q", tc.eventType, got, tc.expected)
			}
		})
	}
}

func TestNamingMakeSSERoutePath(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{
			name:      "dotted event type",
			eventType: "de.telekom.test.v1",
			expected:  "/horizon/sse/v1/de.telekom.test.v1",
		},
		{
			name:      "already hyphenated",
			eventType: "simple-event",
			expected:  "/horizon/sse/v1/simple-event",
		},
		{
			name:      "uppercase letters get lowered",
			eventType: "DE.Telekom.Test.V1",
			expected:  "/horizon/sse/v1/de.telekom.test.v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeSSERoutePath(tc.eventType)
			if got != tc.expected {
				t.Errorf("makeSSERoutePath(%q) = %q, want %q", tc.eventType, got, tc.expected)
			}
		})
	}
}

func TestNamingMakeCallbackRouteName(t *testing.T) {
	tests := []struct {
		name     string
		zoneName string
		expected string
	}{
		{
			name:     "standard zone name",
			zoneName: "zone-a",
			expected: "callback--zone-a",
		},
		{
			name:     "zone name with multiple segments",
			zoneName: "eu-west-1",
			expected: "callback--eu-west-1",
		},
		{
			name:     "empty zone name",
			zoneName: "",
			expected: "callback--",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeCallbackRouteName(tc.zoneName)
			if got != tc.expected {
				t.Errorf("makeCallbackRouteName(%q) = %q, want %q", tc.zoneName, got, tc.expected)
			}
		})
	}
}

func TestNamingMakeCallbackRoutePath(t *testing.T) {
	tests := []struct {
		name     string
		zoneName string
		expected string
	}{
		{
			name:     "standard zone name",
			zoneName: "zone-a",
			expected: "/horizon-zone-a/callback/v1",
		},
		{
			name:     "zone name with multiple segments",
			zoneName: "eu-west-1",
			expected: "/horizon-eu-west-1/callback/v1",
		},
		{
			name:     "empty zone name",
			zoneName: "",
			expected: "/horizon/callback/v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeCallbackRoutePath(tc.zoneName)
			if got != tc.expected {
				t.Errorf("makeCallbackRoutePath(%q) = %q, want %q", tc.zoneName, got, tc.expected)
			}
		})
	}
}
