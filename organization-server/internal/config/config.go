// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	commonconfig "github.com/telekom/controlplane/common-server/pkg/config"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

// ServerConfig holds all configuration for the organization-server facade.
type ServerConfig struct {
	commonconfig.BaseConfig `mapstructure:",squash"`
	CPAPI                   UpstreamConfig `mapstructure:"cpapi"`
	Rover                   UpstreamConfig `mapstructure:"rover"`
}

// UpstreamConfig describes an in-cluster upstream reached on its internal
// listener with a projected ServiceAccount token.
type UpstreamConfig struct {
	// Endpoint is the upstream's internal (k8s-authz) URL.
	Endpoint string `mapstructure:"endpoint"`
	// TokenFilePath is the projected ServiceAccount token for that upstream's
	// audience. The kubelet rotates the file; the client re-reads it on expiry.
	TokenFilePath string `mapstructure:"tokenFilePath"`
	// CaFilePath is the CA bundle used to verify the upstream's TLS cert.
	// Empty means system default CAs.
	CaFilePath string `mapstructure:"caFilePath"`
}

// LoadConfig loads the server configuration from an optional YAML file,
// overlaid with environment variables, on top of DefaultConfig, then validates
// the listener config fail-closed.
func LoadConfig(path string) *ServerConfig {
	cfg := commonconfig.LoadOrDie(path, DefaultConfig())
	if err := cfg.Listeners.Validate(); err != nil {
		panic(fmt.Errorf("validating listeners config: %w", err))
	}
	return cfg
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		BaseConfig: commonconfig.BaseConfig{
			Log: commonconfig.LogConfig{
				Encoding: "json",
				Level:    "info",
			},
			// Default TLS cert/key paths. A tls block in the config file
			// overrides these; empty cert/key downgrades to plain HTTP (dev only).
			TLS: &cserver.TLSFileConfig{
				Cert: "/etc/tls/tls.crt",
				Key:  "/etc/tls/tls.key",
			},
			// ponytail: external JWT listener only. An internal k8s listener
			// would bypass the facade's IdentityExtraction, leaving handlers
			// without a BusinessContext; add one when in-cluster callers need it.
			Listeners: commonconfig.ListenersConfig{
				External: &cserver.ListenerConfig{
					Address: ":8443",
					JWT: &security.JWTConfig{
						Mode:           security.ModeJWT,
						TrustedIssuers: []string{},
						ScopePrefix:    "tardis:",
					},
				},
			},
		},
		CPAPI: UpstreamConfig{ // #nosec G101 -- path to projected token, not a credential
			// Internal (k8s-authz) listener of controlplane-api.
			Endpoint:      "https://controlplane-api.controlplane-system.svc.cluster.local:9443/graphql/query",
			TokenFilePath: "/var/run/secrets/cpapi/token",
			CaFilePath:    "/var/run/secrets/trust-bundle/trust-bundle.pem",
		},
		Rover: UpstreamConfig{ // #nosec G101 -- path to projected token, not a credential
			// Internal (k8s-authz) listener of rover-server.
			Endpoint:      "https://rover-server-service.controlplane-system.svc.cluster.local:9443",
			TokenFilePath: "/var/run/secrets/rover/token",
			CaFilePath:    "/var/run/secrets/trust-bundle/trust-bundle.pem",
		},
	}
}
