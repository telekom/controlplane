// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/telekom/controlplane/common-server/pkg/middleware/k8s"
)

type BackendConfig struct {
	Type   string            `yaml:"type"`
	Config map[string]string `yaml:",inline"`
}

func (c BackendConfig) Get(key string) string {
	if c.Config == nil {
		return ""
	}
	return c.Config[key]
}

func (c BackendConfig) GetDefault(key string, defaultValue string) string {
	if c.Config == nil {
		return defaultValue
	}
	if value, ok := c.Config[key]; ok {
		return value
	}
	return defaultValue
}

type SecurityConfig struct {
	Enabled        bool                      `yaml:"enabled"`
	TrustedIssuers []string                  `yaml:"trusted_issuers"`
	JWKSetURLs     []string                  `yaml:"jwk_set_urls"`
	AccessConfig   []k8s.ServiceAccessConfig `yaml:"access_config"`
}

type ServerConfig struct {
	Security SecurityConfig `yaml:"security"`
	Backend  BackendConfig  `yaml:"backend"`
}

func ReadConfig(r io.Reader) (*ServerConfig, error) {
	cfg := DefaultConfig()
	content, err := io.ReadAll(r)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Security: SecurityConfig{
			Enabled: true,
		},
	}
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
