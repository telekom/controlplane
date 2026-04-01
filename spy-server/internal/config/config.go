// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Address       string              `json:"address"`
	Security      SecurityConfig      `json:"security"`
	Log           LogConfig           `json:"log"`
	SecretManager SecretManagerConfig `json:"secretManager" yaml:"secretManager" mapstructure:"secretManager"`
	// Secrets maps CRD kind names to the JSON field paths within that kind
	// that may contain secret-manager placeholders (e.g. "$<secret-id>").
	// Only used when SecretManager.Enabled is true.
	Secrets map[string][]string `json:"secrets" yaml:"secrets" mapstructure:"secrets"`
}

// SecretManagerConfig controls whether secret-manager resolution is active.
type SecretManagerConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
}

type SecurityConfig struct {
	Enabled        bool `json:"enabled"`
	LMS            LMSConfig
	TrustedIssuers []string `yaml:"trustedIssuers" json:"trustedIssuers"`
	DefaultScope   string   `yaml:"defaultScope" json:"defaultScope"`
	ScopePrefix    string   `yaml:"scopePrefix" json:"scopePrefix"`
}

type LMSConfig struct {
	BasePath string `json:"basePath"`
}

type LogConfig struct {
	Encoding string `json:"encoding"`
	Level    string `json:"level"`
}

func LoadConfig() (*ServerConfig, error) {
	setDefaults()

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var config ServerConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("address", ":8080")

	// Logging
	viper.SetDefault("log.encoding", "json")
	viper.SetDefault("log.level", "info")

	// Security
	viper.SetDefault("security.enabled", true)
	viper.SetDefault("security.trustedIssuers", []string{})
	viper.SetDefault("security.defaultScope", "tardis:user:read")
	viper.SetDefault("security.scopePrefix", "tardis:")
	// LMS
	viper.SetDefault("security.lms.basePath", "")

	// Database
	viper.SetDefault("database.filepath", "")
	viper.SetDefault("database.reduceMemory", false)

	// Informer
	viper.SetDefault("informer.disableCache", true)

	// Secret manager feature flag — set secretManager.enabled: false to disable.
	viper.SetDefault("secretManager.enabled", true)

	// Secret paths per CRD kind — overridable via config file or env vars.
	viper.SetDefault("secrets", map[string][]string{
		"ApiSubscription": {
			"spec.security.m2m.client.clientSecret",
			"spec.security.m2m.basic.password",
		},
		"ApiExposure": {
			"spec.security.m2m.externalIDP.client.clientSecret",
			"spec.security.m2m.externalIDP.basic.password",
			"spec.security.m2m.basic.password",
		},
	})
}
