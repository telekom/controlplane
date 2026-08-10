// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"

	commonconfig "github.com/telekom/controlplane/common-server/pkg/config"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

type ServerConfig struct {
	commonconfig.BaseConfig `mapstructure:",squash"`
	FileManager             FileManagerConfig `mapstructure:"fileManager"`
	OasLinting              OasLintingConfig  `mapstructure:"oasLinting"`
	Database                DatabaseConfig    `mapstructure:"database"`
	Informer                InformerConfig    `mapstructure:"informer"`
	Migration               MigrationConfig   `mapstructure:"migration"`
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

type MigrationConfig struct {
	Active bool `mapstructure:"active"`
}

type OasLintingConfig struct {
	// ErrorMessage is a template for the message returned to clients when linting fails.
	// Supports {{.RulesetName}} and {{.DashboardURL}} as placeholders.
	ErrorMessage string `mapstructure:"errorMessage"`
	// Timeout is the maximum duration to wait for the linter service to respond.
	Timeout time.Duration `mapstructure:"timeout"`
	// URL is the base URL of the OAS linter service.
	URL string `mapstructure:"url"`
	// DashboardURL is a URL template for linking to scan results in the linter UI.
	// Supports {{.LinterId}} and {{.RulesetName}} as placeholders.
	DashboardURL string `mapstructure:"dashboardURL"`
	// SkipTLS disables TLS certificate verification for the linter HTTP client.
	SkipTLS bool `mapstructure:"skipTLS"`
}

type FileManagerConfig struct {
	SkipTLS bool `mapstructure:"skipTLS"`
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
			// External JWT listener on :8443 (HTTPS) plus an internal k8s
			// listener on :9443 for in-cluster callers. Empty accessConfig =
			// any authenticated in-cluster SA; in-cluster issuer auto-discovered.
			Listeners: commonconfig.ListenersConfig{
				External: &cserver.ListenerConfig{
					Address: ":8443",
					JWT: &security.JWTConfig{
						Mode:           security.ModeJWT,
						TrustedIssuers: []string{},
						DefaultScope:   "tardis:team:all",
						ScopePrefix:    "tardis:",
					},
				},
				Internal: &cserver.ListenerConfig{
					Address: ":9443",
					K8s: &cserver.K8sConfig{
						Audience: "rover-server",
					},
				},
			},
		},
		FileManager: FileManagerConfig{
			SkipTLS: true,
		},
		OasLinting: OasLintingConfig{
			ErrorMessage: "Linter scan result contains errors for {{.RulesetName}} ruleset. {{.DashboardURL}}",
			Timeout:      55 * time.Second, // must be below the 60s gateway timeout to allow graceful error handling instead of a raw 504
			SkipTLS:      false,
		},
		Informer: InformerConfig{
			DisableCache: true, // see common-server docs
		},
	}
}
