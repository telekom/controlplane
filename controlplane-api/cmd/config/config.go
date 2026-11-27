// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"

	commonconfig "github.com/telekom/controlplane/common-server/pkg/config"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	filemanager "github.com/telekom/controlplane/file-manager/api"
)

type ServerConfig struct {
	commonconfig.BaseConfig `mapstructure:",squash"`
	Database                DatabaseConfig     `mapstructure:"database"`
	GraphQL                 GraphQLConfig      `mapstructure:"graphql"`
	Kubernetes              KubernetesConfig   `mapstructure:"kubernetes"`
	FileManager             FileManagerConfig  `mapstructure:"fileManager"`
	FileDownload            FileDownloadConfig `mapstructure:"fileDownload"`
	RoverServer             RoverServerConfig  `mapstructure:"roverServer"`
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

// FileManagerConfig holds the configuration of the internal file-manager
// client used by the file download proxy. The BaseURL is the internal base URL
// of the file-manager API, including the /api prefix (for example
// https://file-manager.example.svc/api). TokenFilePath points to the projected
// service-account token (audience file-manager) used to authenticate to it.
type FileManagerConfig struct {
	BaseURL       string `mapstructure:"baseUrl"`
	TokenFilePath string `mapstructure:"tokenFilePath"`
}

// FileDownloadConfig holds the configuration for the public file download
// endpoint. PublicBaseURL is the externally reachable base URL of the
// controlplane-api, used to build specification download URLs.
type FileDownloadConfig struct {
	PublicBaseURL string `mapstructure:"publicBaseUrl"`
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
			Enabled:     true,
			Environment: "poc", // TODO: for now, this is fine. Needs to be refined later
		},
		FileManager: FileManagerConfig{
			BaseURL:       "https://file-manager.controlplane-system.svc.cluster.local/api",
			TokenFilePath: filemanager.TokenFilePath,
		},
		FileDownload: FileDownloadConfig{},
		RoverServer: RoverServerConfig{
			BaseURL:       "https://rover-server-service.controlplane-system.svc.cluster.local:9443",
			TokenFilePath: "/var/run/secrets/rover/token",
			CaFilePath:    "/var/run/secrets/trust-bundle/trust-bundle.pem",
		},
	}
}

// validateAbsoluteURL checks that value is empty or an absolute URL with a
// scheme and a host.
func validateAbsoluteURL(field, value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", field, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s %q must be an absolute URL with scheme and host", field, value)
	}
	return nil
}

// Validate checks that the base URL is empty or an absolute URL with a
// scheme and a host.
func (c FileManagerConfig) Validate() error {
	return validateAbsoluteURL("baseUrl", c.BaseURL)
}

// Validate checks that the public base URL is empty or an absolute URL with a
// scheme and a host.
func (c FileDownloadConfig) Validate() error {
	return validateAbsoluteURL("publicBaseUrl", c.PublicBaseURL)
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
	if err := cfg.FileDownload.Validate(); err != nil {
		panic(fmt.Errorf("validating fileDownload config: %w", err))
	}
	return cfg
}
