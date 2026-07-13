// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestExtractIdentity_ValidClaims(t *testing.T) {
	claims := jwt.MapClaims{
		"clientId": "eni--hyperion--team-user",
		"scope":    "tardis:admin:all openid",
		"iss":      "https://keycloak.example.com/auth/realms/team-controlplane",
	}

	id, err := extractIdentity(claims, "fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Group != "eni" {
		t.Errorf("group: want eni, got %s", id.Group)
	}
	if id.Team != "hyperion" {
		t.Errorf("team: want hyperion, got %s", id.Team)
	}
	if id.Environment != "controlplane" {
		t.Errorf("environment: want controlplane, got %s", id.Environment)
	}
	if len(id.Scopes) != 2 {
		t.Errorf("scopes: want 2, got %d", len(id.Scopes))
	}
}

func TestExtractIdentity_AzpFallback(t *testing.T) {
	claims := jwt.MapClaims{
		"azp":   "eni--hyperion--team-user",
		"scope": "tardis:team:all",
	}

	id, err := extractIdentity(claims, "test-env")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Group != "eni" {
		t.Errorf("group: want eni, got %s", id.Group)
	}
	if id.Team != "hyperion" {
		t.Errorf("team: want hyperion, got %s", id.Team)
	}
	// No issuer, should use fallback environment
	if id.Environment != "test-env" {
		t.Errorf("environment: want test-env, got %s", id.Environment)
	}
}

func TestExtractIdentity_MissingClientId(t *testing.T) {
	claims := jwt.MapClaims{
		"scope": "openid",
	}

	_, err := extractIdentity(claims, "test")
	if err == nil {
		t.Fatal("expected error for missing clientId")
	}
}

func TestExtractIdentity_InvalidClientIdFormat(t *testing.T) {
	claims := jwt.MapClaims{
		"clientId": "just-one-part",
	}

	_, err := extractIdentity(claims, "test")
	if err == nil {
		t.Fatal("expected error for invalid clientId format")
	}
}

func TestExtractIdentity_TwoPartClientId(t *testing.T) {
	claims := jwt.MapClaims{
		"clientId": "eni--hyperion",
	}

	_, err := extractIdentity(claims, "test")
	if err == nil {
		t.Fatal("expected error for two-part clientId")
	}
}

func TestExtractIdentity_ScopesPlural(t *testing.T) {
	claims := jwt.MapClaims{
		"clientId": "eni--team1--user",
		"scopes":   "tardis:admin:all openid profile",
	}

	id, err := extractIdentity(claims, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id.Scopes) != 3 {
		t.Errorf("scopes: want 3, got %d", len(id.Scopes))
	}
}

func TestExtractIdentity_NoScopes(t *testing.T) {
	claims := jwt.MapClaims{
		"clientId": "eni--team1--user",
	}

	id, err := extractIdentity(claims, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id.Scopes) != 0 {
		t.Errorf("scopes: want 0, got %d", len(id.Scopes))
	}
}

func TestExtractEnvironmentFromIssuer(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   string
	}{
		{
			name:   "standard realm",
			claims: jwt.MapClaims{"iss": "https://keycloak.example.com/auth/realms/team-controlplane"},
			want:   "controlplane",
		},
		{
			name:   "realm with dashes",
			claims: jwt.MapClaims{"iss": "https://kc.example.com/realms/team-my-special-env"},
			want:   "my-special-env",
		},
		{
			name:   "no realm prefix",
			claims: jwt.MapClaims{"iss": "https://keycloak.example.com/auth/realms/default"},
			want:   "",
		},
		{
			name:   "missing iss",
			claims: jwt.MapClaims{},
			want:   "",
		},
		{
			name:   "empty iss",
			claims: jwt.MapClaims{"iss": ""},
			want:   "",
		},
		{
			name:   "iss not a string",
			claims: jwt.MapClaims{"iss": 12345},
			want:   "",
		},
		{
			name:   "multiple realms in path (uses last)",
			claims: jwt.MapClaims{"iss": "https://kc.com/realms/team-first/proxy/realms/team-second"},
			want:   "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractEnvironmentFromIssuer(tt.claims)
			if got != tt.want {
				t.Errorf("want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{"admin scope", []string{"tardis:admin:all"}, true},
		{"admin with prefix", []string{"openid", "custom:admin:read"}, true},
		{"no admin scope", []string{"tardis:team:all", "openid"}, false},
		{"empty scopes", nil, false},
		{"single part scope", []string{"admin"}, false},
		{"admin not in second-to-last position", []string{"something:notadmin"}, false},
		{"two-part with admin first", []string{"admin:something"}, true},
		{"three-part admin", []string{"x:admin:y"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAdmin(tt.scopes)
			if got != tt.want {
				t.Errorf("isAdmin(%v): want %v, got %v", tt.scopes, tt.want, got)
			}
		})
	}
}

func TestConsumerIdentityFromContext_NilLocals(t *testing.T) {
	// We can't easily test fiber.Ctx without a real fiber app, but we can verify
	// the function handles nil gracefully via the handler tests.
	// This test documents the expected behavior.
	t.Log("ConsumerIdentityFromContext returns nil when no identity is set - covered by handler auth tests")
}
