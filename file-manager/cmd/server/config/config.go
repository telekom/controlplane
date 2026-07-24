// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	commonconfig "github.com/telekom/controlplane/common-server/pkg/config"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
)

type BackendConfig struct {
	Type   string            `mapstructure:"type"`
	Config map[string]string `mapstructure:",remain"`
}

func (c BackendConfig) Get(key string) string {
	if c.Config == nil {
		return ""
	}
	return c.Config[key]
}

func (c BackendConfig) GetDefault(key, defaultValue string) string {
	if c.Config == nil {
		return defaultValue
	}
	if value, ok := c.Config[key]; ok {
		return value
	}
	return defaultValue
}

type ServerConfig struct {
	commonconfig.BaseConfig `mapstructure:",squash"`
	Backend                 BackendConfig `mapstructure:"backend"`
}

func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		BaseConfig: commonconfig.BaseConfig{
			// Default TLS cert/key paths. A tls block in the config file
			// overrides these; setting them empty in the file downgrades to
			// plain HTTP (dev only).
			TLS: &cserver.TLSFileConfig{
				Cert: "/etc/tls/tls.crt",
				Key:  "/etc/tls/tls.key",
			},
			// Default internal listener: pure-k8s auth on :8443. Omitting
			// trustedIssuers means inCluster auto-discovery; empty accessConfig
			// on the internal listener allows any authenticated in-cluster SA.
			Listeners: commonconfig.ListenersConfig{
				Internal: &cserver.ListenerConfig{
					Address: ":8443",
					K8s: &cserver.K8sConfig{
						Audience: "file-manager",
					},
				},
			},
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
