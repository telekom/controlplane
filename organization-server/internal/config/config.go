// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import "os"

// Config holds all configuration for the organization-server facade.
type Config struct {
	// Port is the HTTP listen port.
	Port string

	// CPAPIEndpoint is the GraphQL endpoint of controlplane-api.
	CPAPIEndpoint string

	// CPAPIInsecure skips TLS certificate verification for CP API calls (local dev only).
	CPAPIInsecure bool

	// RoverEndpoint is the base URL of rover-server.
	RoverEndpoint string

	// OAuthTokenURL is the token endpoint for client_credentials grant.
	OAuthTokenURL string

	// OAuthClientID is the facade's OAuth client ID.
	OAuthClientID string

	// OAuthClientSecret is the facade's OAuth client secret.
	OAuthClientSecret string

	// LogLevel controls log verbosity ("debug" or "info").
	LogLevel string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:              envOrDefault("PORT", "8080"),
		CPAPIEndpoint:     envOrDefault("CPAPI_ENDPOINT", "http://controlplane-api.controlplane-system.svc.cluster.local/graphql/query"),
		CPAPIInsecure:     os.Getenv("CPAPI_INSECURE") == "true",
		RoverEndpoint:     envOrDefault("ROVER_ENDPOINT", "http://rover-server.controlplane-system.svc.cluster.local"),
		OAuthTokenURL:     os.Getenv("OAUTH_TOKEN_URL"),
		OAuthClientID:     os.Getenv("OAUTH_CLIENT_ID"),
		OAuthClientSecret: os.Getenv("OAUTH_CLIENT_SECRET"),
		LogLevel:          envOrDefault("LOG_LEVEL", "info"),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
