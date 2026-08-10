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

type ServerConfig struct {
	commonconfig.BaseConfig `mapstructure:",squash"`
	Database                DatabaseConfig `mapstructure:"database"`
	Informer                InformerConfig `mapstructure:"informer"`
}

type DatabaseConfig struct {
	// Filepath is the on-disk store path; empty means in-memory only.
	Filepath string `mapstructure:"filepath"`
	// ReduceMemory trades memory for CPU; see common-server docs.
	ReduceMemory bool `mapstructure:"reduceMemory"`
}

type InformerConfig struct {
	// DisableCache disables the informer cache; see common-server docs.
	DisableCache bool `mapstructure:"disableCache"`
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
			// One external JWT listener on :8080, plain HTTP (TLS nil). The
			// internal k8s listener is opt-in via config (add a
			// listeners.internal.k8s block); an empty k8s block auto-uses the
			// local cluster issuer.
			Listeners: commonconfig.ListenersConfig{
				External: &cserver.ListenerConfig{
					Address: ":8080",
					JWT: &security.JWTConfig{
						Mode:           security.ModeJWT,
						TrustedIssuers: []string{},
						DefaultScope:   "tardis:user:read",
						ScopePrefix:    "tardis:",
					},
				},
			},
		},
		Informer: InformerConfig{
			DisableCache: true, // see common-server docs
		},
	}
}
