// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"strings"

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
	RoverServer             RoverServerConfig `mapstructure:"roverServer"`
}

type KubernetesConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Kubeconfig string `mapstructure:"kubeconfig"` // optional, defaults to in-cluster config
	// Environments served by this instance: a list, or a comma-separated string.
	// More than one requires graphql.perEnvironmentPathPrefix and database.urlTemplate.
	Environments []string `mapstructure:"environments"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
	// URLTemplate replaces URL; EnvironmentPlaceholder is replaced by each environment.
	URLTemplate string `mapstructure:"urlTemplate"`
}

// EnvironmentPlaceholder marks the environment in database.urlTemplate.
const EnvironmentPlaceholder = "<env>"

// URLFor returns the database URL of the given environment.
func (c DatabaseConfig) URLFor(environment string) string {
	if c.URLTemplate == "" {
		return c.URL
	}
	return strings.ReplaceAll(c.URLTemplate, EnvironmentPlaceholder, environment)
}

type GraphQLConfig struct {
	PlaygroundEnabled bool `mapstructure:"playgroundEnabled"`
	// PerEnvironmentPathPrefix serves every environment of kubernetes.environments
	// under /<environment>/graphql from the database named after it.
	PerEnvironmentPathPrefix bool `mapstructure:"perEnvironmentPathPrefix"`
}

// FileManagerConfig holds the configuration for constructing specification
// download URLs. The BaseURL is the base URL of the file-manager API, including
// the /api prefix (for example https://file-manager.example.svc/api).
type FileManagerConfig struct {
	BaseURL string `mapstructure:"baseUrl"`
}

// RoverServerConfig holds the configuration for the rover-server integration
// used for team resource pre-deletion checks.
type RoverServerConfig struct {
	BaseURL       string `mapstructure:"baseUrl"`
	TokenFilePath string `mapstructure:"tokenFilePath"`
	CaFilePath    string `mapstructure:"caFilePath"`
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
			// External JWT listener on :8443 (secure by default) plus an
			// internal k8s listener on :9443 for in-cluster callers. Empty
			// accessConfig = any authenticated in-cluster SA; in-cluster issuer
			// auto-discovered.
			Listeners: commonconfig.ListenersConfig{
				External: &cserver.ListenerConfig{
					Address: ":8443",
					JWT: &security.JWTConfig{
						Mode: security.ModeJWT,
					},
				},
				Internal: &cserver.ListenerConfig{
					Address: ":9443",
					K8s: &cserver.K8sConfig{
						Audience: "controlplane-api",
					},
				},
			},
		},
		Kubernetes: KubernetesConfig{
			Enabled:      true,
			Environments: []string{"poc"},
		},
		FileManager: FileManagerConfig{
			BaseURL: "https://file-manager.controlplane-system.svc.cluster.local/api",
		},
		RoverServer: RoverServerConfig{
			BaseURL:       "https://rover-server-service.controlplane-system.svc.cluster.local:9443",
			TokenFilePath: "/var/run/secrets/rover/token",
			CaFilePath:    "/var/run/secrets/trust-bundle/trust-bundle.pem",
		},
	}
}

// Validate checks that the base URL is empty or an absolute URL with a
// scheme and a host.
func (c FileManagerConfig) Validate() error {
	if c.BaseURL == "" {
		return nil
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("parsing baseUrl: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("baseUrl %q must be an absolute URL with scheme and host", c.BaseURL)
	}
	return nil
}

// GetConfigOrDie loads the server configuration from an optional YAML file,
// overlaid with environment variables, on top of DefaultConfig, then validates
// the listener and file-manager config fail-closed.
func GetConfigOrDie(filepath string) *ServerConfig {
	cfg := commonconfig.LoadOrDie(filepath, DefaultConfig())
	if err := cfg.Listeners.Validate(); err != nil {
		panic(fmt.Errorf("validating listeners config: %w", err))
	}
	if err := cfg.FileManager.Validate(); err != nil {
		panic(fmt.Errorf("validating fileManager config: %w", err))
	}
	if err := validateEnvironments(cfg); err != nil {
		panic(fmt.Errorf("validating environment config: %w", err))
	}
	return cfg
}

func validateEnvironments(cfg *ServerConfig) error {
	db := cfg.Database
	several := len(cfg.Kubernetes.Environments) > 1
	if len(cfg.Kubernetes.Environments) == 0 {
		return fmt.Errorf("kubernetes.environments must list at least one environment")
	}

	// url always has a default, so only an overridden url conflicts.
	if db.URLTemplate != "" && db.URL != DefaultConfig().Database.URL {
		return fmt.Errorf("database.url and database.urlTemplate are mutually exclusive")
	}
	if db.URLTemplate != "" && !strings.Contains(db.URLTemplate, EnvironmentPlaceholder) {
		return fmt.Errorf("database.urlTemplate must contain %s", EnvironmentPlaceholder)
	}
	if several && !cfg.GraphQL.PerEnvironmentPathPrefix {
		return fmt.Errorf("several environments in kubernetes.environments require graphql.perEnvironmentPathPrefix")
	}
	if several && db.URLTemplate == "" {
		return fmt.Errorf("several environments in kubernetes.environments require database.urlTemplate")
	}
	return nil
}
