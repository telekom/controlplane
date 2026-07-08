// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"testing"
)

func TestIsFullAccessScope(t *testing.T) {
	tests := []struct {
		scope string
		want  bool
	}{
		{"tardis:admin:all", true},
		{"tardis:admin:read", true},
		{"tardis:hub:all", true},
		{"tardis:team:all", true},
		{"tardis:admin:obfuscated", false},
		{"tardis:hub:obfuscated", false},
		{"openid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			if got := isFullAccessScope(tt.scope); got != tt.want {
				t.Errorf("isFullAccessScope(%q) = %v, want %v", tt.scope, got, tt.want)
			}
		})
	}
}

func TestRedactSensitiveFields_SingleObject(t *testing.T) {
	input := `{"name":"team1","clientId":"abc","clientSecret":"secret123","teamToken":"tok456","email":"a@b.c"}`
	result := redactSensitiveFields([]byte(input))
	if result == nil {
		t.Fatal("expected redaction to occur")
	}

	s := string(result)
	if contains(s, "clientSecret") {
		t.Error("clientSecret should be removed")
	}
	if contains(s, "teamToken") {
		t.Error("teamToken should be removed")
	}
	if !contains(s, "clientId") {
		t.Error("clientId should be preserved")
	}
	if !contains(s, "team1") {
		t.Error("name should be preserved")
	}
}

func TestRedactSensitiveFields_PaginatedResponse(t *testing.T) {
	input := `{"items":[{"name":"t1","clientSecret":"s1","teamToken":"tok1"},{"name":"t2","clientSecret":"s2"}],"totalCount":2}`
	result := redactSensitiveFields([]byte(input))
	if result == nil {
		t.Fatal("expected redaction to occur")
	}

	s := string(result)
	if contains(s, "clientSecret") {
		t.Error("clientSecret should be removed from items")
	}
	if contains(s, "teamToken") {
		t.Error("teamToken should be removed from items")
	}
	if !contains(s, "t1") || !contains(s, "t2") {
		t.Error("item names should be preserved")
	}
}

func TestRedactSensitiveFields_NoSensitiveFields(t *testing.T) {
	input := `{"name":"hub1","displayName":"Hub One"}`
	result := redactSensitiveFields([]byte(input))
	if result != nil {
		t.Error("expected nil (no redaction needed)")
	}
}

func TestRedactSensitiveFields_Array(t *testing.T) {
	input := `[{"name":"t1","clientSecret":"s1"},{"name":"t2","teamToken":"tok2"}]`
	result := redactSensitiveFields([]byte(input))
	if result == nil {
		t.Fatal("expected redaction to occur")
	}

	s := string(result)
	if contains(s, "clientSecret") || contains(s, "teamToken") {
		t.Error("sensitive fields should be removed")
	}
}

func contains(s, substr string) bool {
	return s != "" && substr != "" && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
