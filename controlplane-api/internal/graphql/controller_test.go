// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"testing"
)

func Test_decodeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain ASCII", "John Doe", "John Doe"},
		{"encoded space", "John%20Doe", "John Doe"},
		{"encoded non-ASCII", "John%20D%C3%B6e", "John Döe"},
		{"fully encoded email", "user%40example.com", "user@example.com"},
		{"empty string", "", ""},
		{"invalid encoding falls back", "%zz-invalid", "%zz-invalid"},
		{"encoded comma-separated roles", "role%20one,role%20two", "role one,role two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeHeader(tt.input)
			if got != tt.want {
				t.Errorf("decodeHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
