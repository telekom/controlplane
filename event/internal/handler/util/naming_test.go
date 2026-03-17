// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamingConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "MeshClientName",
			got:      MeshClientName,
			expected: "eventstore",
		},
		{
			name:     "AdminClientName",
			got:      AdminClientName,
			expected: "admin--controlplane-client",
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
	tests := []struct {
		name        string
		eventConfig *eventv1.EventConfig
		expected    string
	}{
		{
			name:        "with nil EventConfig",
			eventConfig: nil,
			expected:    "publish",
		},
		{
			name: "with valid EventConfig",
			eventConfig: &eventv1.EventConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			},
			expected: "publish",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makePublishRouteName(tc.eventConfig)
			if got != tc.expected {
				t.Errorf("makePublishRouteName() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestNamingMakePublishRoutePath(t *testing.T) {
	tests := []struct {
		name     string
		zoneName string
		expected string
	}{
		{
			name:     "standard zone name",
			zoneName: "zone-a",
			expected: "/zone-a/publish/v1",
		},
		{
			name:     "zone name with multiple segments",
			zoneName: "eu-west-1",
			expected: "/eu-west-1/publish/v1",
		},
		{
			name:     "empty zone name",
			zoneName: "",
			expected: "//publish/v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makePublishRoutePath(tc.zoneName)
			if got != tc.expected {
				t.Errorf("makePublishRoutePath(%q) = %q, want %q", tc.zoneName, got, tc.expected)
			}
		})
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
			expected:  "/sse/v1/de.telekom.test.v1",
		},
		{
			name:      "already hyphenated",
			eventType: "simple-event",
			expected:  "/sse/v1/simple-event",
		},
		{
			name:      "uppercase letters get lowered",
			eventType: "DE.Telekom.Test.V1",
			expected:  "/sse/v1/de.telekom.test.v1",
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
			expected: "/zone-a/callback/v1",
		},
		{
			name:     "zone name with multiple segments",
			zoneName: "eu-west-1",
			expected: "/eu-west-1/callback/v1",
		},
		{
			name:     "empty zone name",
			zoneName: "",
			expected: "//callback/v1",
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
