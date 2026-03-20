// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Database DatabaseConfig   `yaml:"database"`
	Server   HTTPServerConfig `yaml:"server"`
	Security SecurityConfig   `yaml:"security"`
	GraphQL  GraphQLConfig    `yaml:"graphql"`
	Log      LogConfig        `yaml:"log"`
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
	Enabled        bool     `yaml:"enabled"`
	TrustedIssuers []string `yaml:"trustedIssuers"`
}

type GraphQLConfig struct {
	PlaygroundEnabled bool `yaml:"playgroundEnabled"`
}

type LogConfig struct {
	Level string `yaml:"level"`
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
			Enabled: false,
		},
		GraphQL: GraphQLConfig{
			PlaygroundEnabled: true,
		},
		Log: LogConfig{
			Level: "info",
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
