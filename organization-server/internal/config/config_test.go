// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that might interfere
	envVars := []string{
		"PORT", "CPAPI_ENDPOINT", "CPAPI_CA_FILE",
		"ROVER_ENDPOINT", "ROVER_ENVIRONMENT", "ROVER_SCOPE_PREFIX",
		"SECURITY_TRUSTEDISSUERS",
		"OAUTH_TOKEN_URL", "OAUTH_CLIENT_ID", "OAUTH_CLIENT_SECRET",
		"LOG_LEVEL",
	}
	for _, v := range envVars {
		t.Setenv(v, "")
		os.Unsetenv(v)
	}

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port: want 8080, got %s", cfg.Port)
	}
	if cfg.RoverEnvironment != "controlplane" {
		t.Errorf("RoverEnvironment: want controlplane, got %s", cfg.RoverEnvironment)
	}
	if cfg.RoverScopePrefix != "tardis" {
		t.Errorf("RoverScopePrefix: want tardis, got %s", cfg.RoverScopePrefix)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: want info, got %s", cfg.LogLevel)
	}
	if len(cfg.TrustedIssuers) != 0 {
		t.Errorf("TrustedIssuers: want empty, got %v", cfg.TrustedIssuers)
	}
	if cfg.OAuthTokenURL != "" {
		t.Errorf("OAuthTokenURL: want empty, got %s", cfg.OAuthTokenURL)
	}
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("CPAPI_ENDPOINT", "http://custom-cpapi/graphql")
	t.Setenv("ROVER_ENDPOINT", "http://custom-rover")
	t.Setenv("ROVER_ENVIRONMENT", "staging")
	t.Setenv("ROVER_SCOPE_PREFIX", "custom")
	t.Setenv("SECURITY_TRUSTEDISSUERS", "https://issuer1.com, https://issuer2.com")
	t.Setenv("OAUTH_TOKEN_URL", "https://token.example.com/token")
	t.Setenv("OAUTH_CLIENT_ID", "my-client")
	t.Setenv("OAUTH_CLIENT_SECRET", "my-secret")
	t.Setenv("LOG_LEVEL", "debug")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Port: want 9090, got %s", cfg.Port)
	}
	if cfg.CPAPIEndpoint != "http://custom-cpapi/graphql" {
		t.Errorf("CPAPIEndpoint: got %s", cfg.CPAPIEndpoint)
	}
	if cfg.RoverEndpoint != "http://custom-rover" {
		t.Errorf("RoverEndpoint: got %s", cfg.RoverEndpoint)
	}
	if cfg.RoverEnvironment != "staging" {
		t.Errorf("RoverEnvironment: got %s", cfg.RoverEnvironment)
	}
	if cfg.RoverScopePrefix != "custom" {
		t.Errorf("RoverScopePrefix: got %s", cfg.RoverScopePrefix)
	}
	if len(cfg.TrustedIssuers) != 2 {
		t.Fatalf("TrustedIssuers: want 2, got %d", len(cfg.TrustedIssuers))
	}
	if cfg.TrustedIssuers[0] != "https://issuer1.com" {
		t.Errorf("TrustedIssuers[0]: got %s", cfg.TrustedIssuers[0])
	}
	if cfg.TrustedIssuers[1] != "https://issuer2.com" {
		t.Errorf("TrustedIssuers[1]: got %s", cfg.TrustedIssuers[1])
	}
	if cfg.OAuthTokenURL != "https://token.example.com/token" {
		t.Errorf("OAuthTokenURL: got %s", cfg.OAuthTokenURL)
	}
	if cfg.OAuthClientID != "my-client" {
		t.Errorf("OAuthClientID: got %s", cfg.OAuthClientID)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %s", cfg.LogLevel)
	}
}

func TestLoad_TrustedIssuers_EmptyEntries(t *testing.T) {
	t.Setenv("SECURITY_TRUSTEDISSUERS", "https://a.com, , https://b.com, ")

	cfg := Load()

	if len(cfg.TrustedIssuers) != 2 {
		t.Fatalf("TrustedIssuers: want 2 (skip empty), got %d: %v", len(cfg.TrustedIssuers), cfg.TrustedIssuers)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_KEY_EXISTS", "custom-value")

	if got := envOrDefault("TEST_KEY_EXISTS", "default"); got != "custom-value" {
		t.Errorf("want custom-value, got %s", got)
	}

	os.Unsetenv("TEST_KEY_MISSING")
	if got := envOrDefault("TEST_KEY_MISSING", "default"); got != "default" {
		t.Errorf("want default, got %s", got)
	}
}
