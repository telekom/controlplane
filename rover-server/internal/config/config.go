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
	Address  string         `json:"address"`
	Security SecurityConfig `json:"security"`
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

func LoadConfig() (*ServerConfig, error) {

	setDefaults()

	viper.AutomaticEnv()
	viper.SetEnvPrefix("ROVER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var config ServerConfig
	if err := viper.Unmarshal(&config); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.address", ":8080")

	// Security
	viper.SetDefault("security.enabled", true)
	viper.SetDefault("security.trustedIssuers", []string{})
	viper.SetDefault("security.defaultScope", "tardis:team:all")
	viper.SetDefault("security.scopePrefix", "tardis:")
	// LMS
	viper.SetDefault("security.lms.basePath", "")
}
