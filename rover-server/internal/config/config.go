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
	Address     string            `json:"address"`
	Security    SecurityConfig    `json:"security"`
	Log         LogConfig         `json:"log"`
	FileManager FileManagerConfig `json:"fileManager"`
	OasLinting  OasLintingConfig  `json:"oasLinting"`
}

type OasLintingConfig struct {
	URL                   string `json:"url"`
	ErrorMessage          string `json:"errorMessage"`
	WhitelistedBasepaths  string `json:"whitelistedBasepaths"`
	WhitelistedCategories string `json:"whitelistedCategories"`
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

type FileManagerConfig struct {
	SkipTLS bool `json:"skipTLS"`
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
	viper.SetDefault("security.defaultScope", "tardis:team:all")
	viper.SetDefault("security.scopePrefix", "tardis:")
	// LMS
	viper.SetDefault("security.lms.basePath", "")

	// FileManager
	viper.SetDefault("fileManager.skipTLS", true)

	// OAS Linting
	viper.SetDefault("oasLinting.url", "")
	viper.SetDefault("oasLinting.errorMessage", "Linter scan result contains errors. Please visit the linter UI for details on the RULESET_NAME_PLACEHOLDER ruleset.")
	viper.SetDefault("oasLinting.whitelistedBasepaths", "")
	viper.SetDefault("oasLinting.whitelistedCategories", "")

	// Database
	viper.SetDefault("database.filepath", "")        // empty string means in-memory only
	viper.SetDefault("database.reduceMemory", false) // see common-server docs

	// Informer
	viper.SetDefault("informer.disableCache", true) // see common-server docs

	// Migration
	viper.SetDefault("migration.active", false)
}
