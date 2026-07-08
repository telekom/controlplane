// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"strings"
)

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

	// RoverEnvironment is the environment claim used when constructing service tokens
	// for rover-server calls (e.g. "controlplane").
	RoverEnvironment string

	// RoverScopePrefix is the scope prefix rover-server expects (e.g. "tardis").
	RoverScopePrefix string

	// TrustedIssuers is a comma-separated list of Keycloak issuer URLs.
	// If empty, security runs in mock mode (tokens parsed but not verified).
	TrustedIssuers []string

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
	var issuers []string
	if v := os.Getenv("SECURITY_TRUSTEDISSUERS"); v != "" {
		for _, iss := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(iss); trimmed != "" {
				issuers = append(issuers, trimmed)
			}
		}
	}

	return &Config{
		Port:              envOrDefault("PORT", "8080"),
		CPAPIEndpoint:     envOrDefault("CPAPI_ENDPOINT", "https://controlplane-api.controlplane-system.svc.cluster.local/graphql/query"),
		CPAPICaFilePath:   envOrDefault("CPAPI_CA_FILE", "/var/run/secrets/trust-bundle/trust-bundle.pem"),
		RoverEndpoint:     envOrDefault("ROVER_ENDPOINT", "http://rover-server.controlplane-system.svc.cluster.local"),
		RoverEnvironment:  envOrDefault("ROVER_ENVIRONMENT", "controlplane"),
		RoverScopePrefix:  envOrDefault("ROVER_SCOPE_PREFIX", "tardis"),
		TrustedIssuers:    issuers,
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
