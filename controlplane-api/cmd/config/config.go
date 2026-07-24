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
	Database                DatabaseConfig    `mapstructure:"database"`
	GraphQL                 GraphQLConfig     `mapstructure:"graphql"`
	Kubernetes              KubernetesConfig  `mapstructure:"kubernetes"`
	FileManager             FileManagerConfig `mapstructure:"fileManager"`
}

type KubernetesConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	Kubeconfig  string `mapstructure:"kubeconfig"`  // optional, defaults to in-cluster config
	Environment string `mapstructure:"environment"` // environment scope for the scoped client
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type GraphQLConfig struct {
	PlaygroundEnabled bool `mapstructure:"playgroundEnabled"`
}

// FileManagerConfig holds the configuration for constructing specification
// download URLs. The BaseURL is the root URL of the file-manager service.
type FileManagerConfig struct {
	BaseURL string `mapstructure:"baseUrl"`
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Database: DatabaseConfig{
			URL: "postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable",
		},
		GraphQL: GraphQLConfig{
			PlaygroundEnabled: true,
		},
		BaseConfig: commonconfig.BaseConfig{
			Log: commonconfig.LogConfig{
				Level: "debug",
			},
			// Default TLS cert/key paths. A tls block in the config file
			// overrides these; empty cert/key downgrades to plain HTTP (dev only).
			TLS: &cserver.TLSFileConfig{
				Cert: "/etc/tls/tls.crt",
				Key:  "/etc/tls/tls.key",
			},
			// controlplane-api is external-only: a single JWT listener on :8443
			// (secure by default). No internal k8s listener.
			Listeners: commonconfig.ListenersConfig{
				External: &cserver.ListenerConfig{
					Address: ":8443",
					JWT: &security.JWTConfig{
						Mode: security.ModeJWT,
					},
				},
			},
		},
		Kubernetes: KubernetesConfig{
			Enabled:     true,
			Environment: "poc", // TODO: for now, this is fine. Needs to be refined later
		},
		FileManager: FileManagerConfig{
			BaseURL: "file-manager.controlplane-system.svc",
		},
	}
}

// GetConfigOrDie loads the server configuration from an optional YAML file,
// overlaid with environment variables, on top of DefaultConfig, then validates
// the listener config fail-closed.
func GetConfigOrDie(filepath string) *ServerConfig {
	cfg := commonconfig.LoadOrDie(filepath, DefaultConfig())
	if err := cfg.Listeners.Validate(); err != nil {
		panic(fmt.Errorf("validating listeners config: %w", err))
	}
	return cfg
}
