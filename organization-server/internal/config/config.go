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

	// CPAPICaFilePath is the path to the CA bundle for verifying CP API's TLS cert.
	// If empty, system default CAs are used (works when cert is publicly trusted).
	CPAPICaFilePath string

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
		CPAPIEndpoint:     envOrDefault("CPAPI_ENDPOINT", "https://controlplane-api.controlplane-system.svc.cluster.local/graphql/query"),
		CPAPICaFilePath:   envOrDefault("CPAPI_CA_FILE", "/var/run/secrets/trust-bundle/trust-bundle.pem"),
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
