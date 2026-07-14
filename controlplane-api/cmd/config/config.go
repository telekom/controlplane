// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

type ServerConfig struct {
	Database    DatabaseConfig    `yaml:"database"`
	Server      HTTPServerConfig  `yaml:"server"`
	Security    SecurityConfig    `yaml:"security"`
	GraphQL     GraphQLConfig     `yaml:"graphql"`
	Log         LogConfig         `yaml:"log"`
	Kubernetes  KubernetesConfig  `yaml:"kubernetes"`
	FileManager FileManagerConfig `yaml:"fileManager"`
}

type KubernetesConfig struct {
	Enabled     bool   `yaml:"enabled"`
	Kubeconfig  string `yaml:"kubeconfig"`  // optional, defaults to in-cluster config
	Environment string `yaml:"environment"` // environment scope for the scoped client
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

type HTTPServerConfig struct {
	Address string    `yaml:"address"`
	TLS     TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

type SecurityConfig struct {
	// Mode controls authentication behaviour: use security.ModeJWT or security.ModeMock.
	Mode           security.Mode `yaml:"mode"`
	TrustedIssuers []string      `yaml:"trustedIssuers"`
}

type GraphQLConfig struct {
	PlaygroundEnabled bool `yaml:"playgroundEnabled"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

// FileManagerConfig holds the configuration for constructing specification
// download URLs. The BaseURL is the root URL of the file-manager service.
type FileManagerConfig struct {
	BaseURL string `yaml:"baseUrl"`
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Database: DatabaseConfig{
			URL: "postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable",
		},
		Server: HTTPServerConfig{
			Address: ":8443",
			TLS: TLSConfig{
				Enabled: true,
				Cert:    "/etc/tls/tls.crt",
				Key:     "/etc/tls/tls.key",
			},
		},
		Security: SecurityConfig{
			Mode: security.ModeJWT,
		},
		GraphQL: GraphQLConfig{
			PlaygroundEnabled: true,
		},
		Log: LogConfig{
			Level: "debug",
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

func ReadConfig(r io.Reader) (*ServerConfig, error) {
	cfg := DefaultConfig()
	content, err := io.ReadAll(r)
	if err != nil {
		return cfg, err
	}
	expanded := os.ExpandEnv(string(content))
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func GetConfigOrDie(filepath string) *ServerConfig {
	if filepath == "" {
		return DefaultConfig()
	}
	file, err := os.OpenFile(filepath, os.O_RDONLY, 0o644) //nolint:gosec
	if err != nil {
		panic(err)
	}
	defer file.Close() //nolint:errcheck
	cfg, err := ReadConfig(file)
	if err != nil {
		panic(err)
	}
	return cfg
}
