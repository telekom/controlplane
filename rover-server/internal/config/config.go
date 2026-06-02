// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"time"

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
	// ErrorMessage is a template for the message returned to clients when linting fails.
	// Supports {{.RulesetName}} and {{.DashboardURL}} as placeholders.
	ErrorMessage string `json:"errorMessage"`
	// Timeout is the maximum duration to wait for the linter service to respond.
	Timeout time.Duration `json:"timeout"`
	// URL is the base URL of the OAS linter service.
	URL string `json:"url"`
	// DashboardURL is a URL template for linking to scan results in the linter UI.
	// Supports {{.LinterId}} and {{.RulesetName}} as placeholders.
	DashboardURL string `json:"dashboardURL"`
	// SkipTLS disables TLS certificate verification for the linter HTTP client.
	SkipTLS bool `json:"skipTLS"`
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
	viper.SetDefault("oasLinting.errorMessage", "Linter scan result contains errors for {{.RulesetName}} ruleset. {{.DashboardURL}}")
	viper.SetDefault("oasLinting.timeout", 55*time.Second) // must be below the 60s gateway timeout to allow graceful error handling instead of a raw 504
	viper.SetDefault("oasLinting.url", "")
	viper.SetDefault("oasLinting.dashboardURL", "") // e.g. https://linter.example.com/scans/{{.LinterId}}
	viper.SetDefault("oasLinting.skipTLS", false)

	// Database
	viper.SetDefault("database.filepath", "")        // empty string means in-memory only
	viper.SetDefault("database.reduceMemory", false) // see common-server docs

	// Informer
	viper.SetDefault("informer.disableCache", true) // see common-server docs

	// Migration
	viper.SetDefault("migration.active", false)
}
